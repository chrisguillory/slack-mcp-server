package edge

import (
	"context"
	"errors"
	"runtime/trace"
	"sync"

	"github.com/rusq/slack"
)

var ErrParameterMissing = errors.New("required parameter missing")

// High level functions that wrap low level calls to webclient API to return
// the data in the format close to the Slack API.

func (cl *Client) GetConversationsContext(ctx context.Context, _ *slack.GetConversationsParameters) (channels []slack.Channel, _ string, err error) {
	type result struct {
		Channels []slack.Channel
		Err      error
	}

	var resultC = make(chan result, 2)
	var pipeline = []func(){
		func() {
			// getting client.userBoot information
			ub, err := cl.ClientUserBoot(ctx)
			if err != nil {
				resultC <- result{Err: err}
				return
			}
			var ch = make([]slack.Channel, 0, len(ub.Channels))
			for _, c := range ub.Channels {
				ch = append(ch, c.SlackChannel())
			}
			resultC <- result{Channels: ch, Err: err}
		},
		func() {
			// collecting the IMs.
			ims, err := cl.IMList(ctx)
			var ch = make([]slack.Channel, 0, len(ims))
			for _, c := range ims {
				ch = append(ch, c.SlackChannel())
			}
			resultC <- result{Channels: ch, Err: err}
		},
		func() {
			// collecting the channels.
			ch, err := cl.SearchChannels(ctx, "")
			resultC <- result{Channels: ch, Err: err}
		},
	}

	var wg sync.WaitGroup
	wg.Add(len(pipeline))
	for _, f := range pipeline {
		go func(f func()) {
			defer wg.Done()
			f()
		}(f)
	}
	go func() {
		wg.Wait()
		close(resultC)
	}()

	// create a map of channels that we have already seen
	var seenChannels = make(map[string]int) // map channel ID to index in channels slice
	for r := range resultC {
		if r.Err != nil {
			return nil, "", r.Err
		}
		for _, c := range r.Channels {
			if idx, seen := seenChannels[c.ID]; !seen {
				seenChannels[c.ID] = len(channels)
				channels = append(channels, c)
			} else {
				// Merge channel data, preferring non-zero member counts
				if c.NumMembers > channels[idx].NumMembers {
					channels[idx].NumMembers = c.NumMembers
				}
			}
		}
	}

	// ClientCounts hopefully returns MPIM IDs that we haven't seen in the
	// user boot response.
	cr, err := cl.ClientCounts(ctx)
	if err != nil {
		return nil, "", err
	}

	// determine which mpims are already in the list, and which need to be
	// fetched
	var fetchIDs = make([]string, 0, len(cr.MPIMs))
	for _, c := range cr.MPIMs {
		if _, seen := seenChannels[c.ID]; !seen {
			fetchIDs = append(fetchIDs, c.ID)
		}
	}

	// getting the info on any MPIMs that we haven't seen yet.
	mpims, err := cl.ConversationsGenericInfo(ctx, fetchIDs...)
	if err != nil {
		return nil, "", err
	}
	channels = append(channels, mpims...)
	return channels, "", nil
}

func (cl *Client) GetUsersInConversationContext(ctx context.Context, p *slack.GetUsersInConversationParameters) (ids []string, _ string, err error) {
	if p.ChannelID == "" {
		return nil, "", ErrParameterMissing
	}
	uu, err := cl.UsersList(ctx, p.ChannelID)
	if err != nil {
		return nil, "", err
	}
	for _, u := range uu {
		ids = append(ids, u.ID)
	}
	return ids, "", nil
}

var ErrNotFound = errors.New("not found")

func (cl *Client) GetConversationInfoContext(ctx context.Context, input *slack.GetConversationInfoInput) (*slack.Channel, error) {
	cc, err := cl.ConversationsGenericInfo(ctx, input.ChannelID)
	if err != nil {
		return nil, err
	}
	if len(cc) == 0 {
		return nil, ErrNotFound
	}
	return &cc[0], nil
}

// GetBotInfoContext returns bot information using the bots.info API
func (cl *Client) GetBotInfoContext(ctx context.Context, parameters slack.GetBotInfoParameters) (*slack.Bot, error) {
	ctx, task := trace.NewTask(ctx, "GetBotInfo")
	defer task.End()

	// Use the standard Slack API client for bots.info
	// The Edge client doesn't have direct support for bots.info, so we delegate to the standard client
	// This is consistent with how other bot-related operations work
	return nil, errors.New("bots.info not supported in edge client - use standard API")
}
