# Comparison: reactions.add vs conversations.create Browser Curls

## Similarities (What Both Have)

### URL Structure
Both have identical query parameter structure:
- `_x_id` - CSRF token (different values per request)
- `_x_csid` - Client session ID  
- `slack_route` - Enterprise Grid routing
- `_x_version_ts` - Version timestamp
- `_x_frontend_build_type=current`
- `_x_desktop_ia=4`
- `_x_gantry=true`
- `fp=39`
- `_x_num_retries=0`

### Headers
Identical headers:
- `content-type: multipart/form-data; boundary=...`
- Same cookie with `d=xoxd-...` token
- Same browser headers (User-Agent, sec-ch-ua, etc.)
- Same origin: `https://app.slack.com`

### Form Structure
Both use multipart form data with:
- `token` field with xoxc token
- `_x_mode=online`
- `_x_sonic=true`
- `_x_app_name=client`
- WebKit boundary format

## Key Differences

### 1. API Endpoint
- **reactions.add**: `https://mainstayio.enterprise.slack.com/api/reactions.add`
- **conversations.create**: `https://mainstayio.enterprise.slack.com/api/conversations.create`

### 2. API-Specific Form Fields

**reactions.add fields:**
```
channel: C09D35MGM7D
timestamp: 1756862180.414819
name: eyes (the emoji)
_x_reason: changeReactionFromUserAction
```

**conversations.create fields:**
```
name: tmp-this-is-a-test-create... (channel name)
validate_name: true
is_private: true
team_id: T08U80K08H4
```

### 3. Edge Client Implementation

**reactions.add (WORKING):**
```go
// From pkg/provider/edge/reactions.go
resp, err := cl.PostForm(ctx, "reactions.add", values(form, true))
```
- Uses standard PostForm
- URL-encoded form data (NOT multipart!)
- No query parameters
- Works perfectly!

**conversations.create (NOT WORKING):**
```go
// Our attempt
resp, err := cl.PostForm(ctx, "conversations.create", values(form, true))
```
- Same pattern as reactions
- But returns: `cannot_create_channel`

## The Mystery

Both browser curls use:
- Multipart form data
- CSRF tokens in URL
- Identical authentication structure

But in Edge client:
- reactions.add works with URL-encoded forms, no CSRF tokens
- conversations.create fails with same approach

## Hypothesis

### Why reactions.add works in Edge client:
1. The Edge client likely has a **different authentication context** than browser
2. reactions.add may be **less restrictive** - allows both browser and API access
3. The Edge client auth may have reaction permissions by default

### Why conversations.create fails in Edge client:
1. Channel creation may be **restricted to browser context only** in Enterprise Grid
2. May require **admin permissions** that Edge client doesn't have
3. Could be blocked at **organizational policy level** for programmatic access
4. The endpoint might **not exist** in the Edge client API surface

## What This Means

The curl commands are nearly identical because they're both from the browser. The difference isn't in the curl structure - it's that:

1. **reactions.add** has a dual implementation:
   - Browser version (multipart + CSRF)
   - Edge API version (URL-encoded, no CSRF)

2. **conversations.create** might only have:
   - Browser version (multipart + CSRF)
   - No Edge API equivalent for Enterprise Grid

## Test to Confirm

We could test if the Edge client implementation difference is the issue:
1. Try reactions.add with multipart form (like curl) - if it fails, confirms Edge needs URL-encoded
2. Check if admin.conversations.create exists in Edge client
3. Look for error details beyond "cannot_create_channel"

## Conclusion

Your confusion is justified! The browser curls are nearly identical, but the Edge client treats these endpoints differently. The issue isn't our implementation - it's that conversations.create might not be available through the Edge client path for Enterprise Grid.