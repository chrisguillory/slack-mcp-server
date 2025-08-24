package edge

import (
	"context"
	"runtime/trace"

	"github.com/slack-go/slack"
)

// conversationsAddReactionForm is the request to reactions.add
type conversationsAddReactionForm struct {
	BaseRequest
	Channel   string `json:"channel"`
	Timestamp string `json:"timestamp"`
	Name      string `json:"name"`
	WebClientFields
}

type conversationsAddReactionResponse struct {
	baseResponse
}

// conversationsRemoveReactionForm is the request to reactions.remove
type conversationsRemoveReactionForm struct {
	BaseRequest
	Channel   string `json:"channel"`
	Timestamp string `json:"timestamp"`
	Name      string `json:"name"`
	WebClientFields
}

type conversationsRemoveReactionResponse struct {
	baseResponse
}

func (cl *Client) AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
	ctx, task := trace.NewTask(ctx, "AddReactionContext")
	defer task.End()
	trace.Logf(ctx, "params", "channel=%v, timestamp=%v, name=%v", item.Channel, item.Timestamp, name)

	form := conversationsAddReactionForm{
		BaseRequest: BaseRequest{
			Token: cl.token,
		},
		Channel:         item.Channel,
		Timestamp:       item.Timestamp,
		Name:            name,
		WebClientFields: webclientReason("changeReactionFromUserAction"),
	}

	resp, err := cl.PostForm(ctx, "reactions.add", values(form, true))
	if err != nil {
		return err
	}

	var r conversationsAddReactionResponse
	return cl.ParseResponse(&r, resp)
}

func (cl *Client) RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error {
	ctx, task := trace.NewTask(ctx, "RemoveReactionContext")
	defer task.End()
	trace.Logf(ctx, "params", "channel=%v, timestamp=%v, name=%v", item.Channel, item.Timestamp, name)

	form := conversationsRemoveReactionForm{
		BaseRequest: BaseRequest{
			Token: cl.token,
		},
		Channel:         item.Channel,
		Timestamp:       item.Timestamp,
		Name:            name,
		WebClientFields: webclientReason("changeReactionFromUserAction"),
	}

	resp, err := cl.PostForm(ctx, "reactions.remove", values(form, true))
	if err != nil {
		return err
	}

	var r conversationsRemoveReactionResponse
	return cl.ParseResponse(&r, resp)
}
