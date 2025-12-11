# Slack Block Kit Templates

This file contains curated Block Kit templates for professional Slack messages. Use these as a starting point when you need to send well-formatted messages.

## Quick Reference

| Template | Use When |
|----------|----------|
| [Status Update](#status-update) | Reporting progress, daily standups, project updates |
| [Alert](#alert) | Urgent notifications, errors, warnings, incidents |
| [Meeting Summary](#meeting-summary) | Recapping meetings, sharing notes and action items |
| [Announcement](#announcement) | News, releases, company updates, celebrations |
| [Request](#request) | Asking for approval, input, or action from others |
| [Report](#report) | Data summaries, metrics, dashboards |
| [Error](#error) | API failures, system errors, graceful degradation |
| [Empty State](#empty-state) | No data available, not configured, first-time setup |
| [Compact Fields](#compact-fields) | Key-value pairs, metadata, side-by-side layout |

---

## Status Update

**Use for:** Progress reports, daily standups, project status

**Text fallback:** Keep it brief - "Status update: [one-line summary]"

```json
[
  {
    "type": "header",
    "text": {"type": "plain_text", "text": "Project Status Update"}
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Project:* Feature X Implementation\n*Status:* :large_green_circle: On Track\n*Owner:* <@U123>"
    }
  },
  {
    "type": "divider"
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Completed this week:*\n• Implemented user authentication\n• Added database migrations\n• Code review completed"
    }
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Up next:*\n• Integration testing\n• Documentation updates\n• Stakeholder demo"
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": "Updated: <!date^1702300800^{date_short} at {time}|Dec 11, 2024>"}
    ]
  }
]
```

**Status indicators:**
- `:large_green_circle:` On Track
- `:large_yellow_circle:` At Risk
- `:red_circle:` Blocked
- `:white_circle:` Not Started

---

## Alert

**Use for:** Urgent notifications, errors, incidents, warnings

**Text fallback:** Include severity and summary - "[SEVERITY] Alert: [description]"

### Critical Alert
```json
[
  {
    "type": "header",
    "text": {"type": "plain_text", "text": ":rotating_light: Critical Alert"}
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Issue:* Database connection failures\n*Impact:* Users unable to log in\n*Started:* <!date^1702300800^{date_short} {time}|2:00 PM>"
    }
  },
  {
    "type": "divider"
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Current Status:*\nEngineering team investigating. Initial assessment suggests connection pool exhaustion.\n\n*Next Update:* 15 minutes"
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": "Incident #1234 | Severity: Critical | <https://statuspage.io|Status Page>"}
    ]
  }
]
```

### Warning/Info Alert
```json
[
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": ":warning: *Scheduled Maintenance*\n\nThe platform will be unavailable on *Saturday, Dec 14* from *2:00 AM - 4:00 AM EST* for database upgrades.\n\nNo action required. Services will resume automatically."
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": "Questions? Contact <#C-ops-channel|ops>"}
    ]
  }
]
```

**Alert emoji prefixes:**
- `:rotating_light:` Critical/Emergency
- `:warning:` Warning
- `:information_source:` Info
- `:white_check_mark:` Resolved

---

## Meeting Summary

**Use for:** Meeting recaps, sharing notes, action items

**Text fallback:** "Meeting summary: [meeting name] - [key outcome]"

```json
[
  {
    "type": "header",
    "text": {"type": "plain_text", "text": "Meeting Summary: Q1 Planning"}
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Date:* December 11, 2024\n*Attendees:* <@U123>, <@U456>, <@U789>\n*Duration:* 45 minutes"
    }
  },
  {
    "type": "divider"
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Key Decisions:*\n• Approved budget for new hire\n• Launch date set for March 1\n• Weekly syncs moving to Tuesdays"
    }
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Action Items:*\n• <@U123> - Draft job description by Friday\n• <@U456> - Update project timeline\n• <@U789> - Send calendar invites for new sync time"
    }
  },
  {
    "type": "divider"
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": ":memo: <https://docs.google.com/doc/123|Full Notes> | :calendar: Next meeting: Dec 18"}
    ]
  }
]
```

---

## Announcement

**Use for:** News, releases, company updates, celebrations

**Text fallback:** "[Announcement] [headline]"

### Feature Release
```json
[
  {
    "type": "header",
    "text": {"type": "plain_text", "text": ":rocket: New Feature Launch"}
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Dark Mode is here!*\n\nWe've heard your feedback - the app now supports dark mode across all platforms."
    }
  },
  {
    "type": "divider"
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*How to enable:*\n1. Go to Settings > Appearance\n2. Select \"Dark\" or \"System default\"\n3. Enjoy the new look!"
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": ":book: <https://docs.example.com|Documentation> | :speech_balloon: <#C-feedback|Share feedback>"}
    ]
  }
]
```

### Team Celebration
```json
[
  {
    "type": "header",
    "text": {"type": "plain_text", "text": ":tada: Congratulations!"}
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "Huge shoutout to <@U123> for closing the biggest deal in company history!\n\n:trophy: *$2.5M ARR* :trophy:\n\nYour persistence and relationship-building made this happen. The whole team is celebrating with you!"
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": "Posted by <@U456> | #wins"}
    ]
  }
]
```

---

## Request

**Use for:** Asking for approval, feedback, input, or action

**Text fallback:** "[Request] [what you need] - [deadline if any]"

### Approval Request
```json
[
  {
    "type": "header",
    "text": {"type": "plain_text", "text": ":memo: Approval Requested"}
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Request:* Budget approval for team offsite\n*Amount:* $5,000\n*Requested by:* <@U123>"
    }
  },
  {
    "type": "divider"
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Details:*\n• 2-day offsite in Austin, TX\n• Team building + Q2 planning\n• 8 attendees\n• <https://docs.google.com/doc/123|Full proposal>"
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": ":clock3: Response needed by: Dec 15 | Reply in thread to approve/discuss"}
    ]
  }
]
```

### Feedback Request
```json
[
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": ":eyes: *Feedback Requested*\n\nI've drafted the Q1 roadmap and would love your input before sharing with leadership.\n\n:page_facing_up: <https://docs.google.com/doc/123|View Draft>"
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": "Please review by EOD Friday | Comment directly in doc or reply here"}
    ]
  }
]
```

---

## Report

**Use for:** Metrics, data summaries, dashboards, weekly reports

**Text fallback:** "[Report] [report name] - [key metric or finding]"

### Metrics Dashboard
```json
[
  {
    "type": "header",
    "text": {"type": "plain_text", "text": ":bar_chart: Weekly Metrics Report"}
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Week of December 9-13, 2024*"
    }
  },
  {
    "type": "divider"
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Revenue*\n:chart_with_upwards_trend: $125,000 (+12% WoW)\n\n*New Users*\n:bust_in_silhouette: 1,247 (+8% WoW)\n\n*Churn Rate*\n:chart_with_downwards_trend: 2.1% (-0.3% WoW)"
    }
  },
  {
    "type": "divider"
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Highlights:*\n• Record signups on Tuesday following ProductHunt launch\n• Support ticket volume down 15%\n• NPS score improved to 72"
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": ":link: <https://dashboard.example.com|Full Dashboard> | Generated automatically"}
    ]
  }
]
```

### Trend Indicators

Use these patterns for showing metric changes:

| Change | Pattern | Example |
|--------|---------|---------|
| Up (good) | `:arrow_up:` or `:chart_with_upwards_trend:` + green text | `+12% :arrow_up:` |
| Down (good) | `:arrow_down:` + context | `2.1% (-0.3%) :arrow_down:` |
| Up (bad) | `:arrow_up:` + warning context | `Churn: 5% :arrow_up: *(above target)*` |
| Down (bad) | `:arrow_down:` + warning | `Revenue: -8% :arrow_down:` |
| No change | `:left_right_arrow:` | `Flat :left_right_arrow:` |

---

## Error

**Use for:** API failures, system errors, graceful degradation, retryable issues

**Text fallback:** "Error: [brief description] - [action needed]"

### Retryable Error
```json
[
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": ":x: *Unable to complete request*\n\nThe external API returned an error. This is usually temporary."
    }
  },
  {
    "type": "divider"
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*What happened:*\nConnection timeout while fetching data from the analytics service.\n\n*What to do:*\nPlease try again in a few minutes. If the issue persists, contact support."
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": "Error ID: `err_7x9k2m` | <https://status.example.com|Service Status>"}
    ]
  }
]
```

### Configuration Error
```json
[
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": ":warning: *Action Required: Missing Configuration*\n\nI couldn't complete your request because the integration isn't fully set up."
    }
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Missing:*\n• API key for Jira integration\n• Webhook URL for notifications\n\n*How to fix:*\nVisit <https://app.example.com/settings|Settings> to complete the setup."
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": "Need help? See <https://docs.example.com/setup|Setup Guide>"}
    ]
  }
]
```

---

## Empty State

**Use for:** No data available, first-time setup, zero results

**Text fallback:** "[Context] No data available - [what to do]"

### No Data Yet
```json
[
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": ":bar_chart: *Weekly Metrics Report*\n\n_No data available for this period._\n\nMetrics will appear here once activity is recorded. Check back after your first users start using the platform."
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": "Data collection started: Dec 10, 2024 | <https://docs.example.com/metrics|Learn about metrics>"}
    ]
  }
]
```

### No Results Found
```json
[
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": ":mag: *Search Results*\n\n_No matching items found for \"quarterly report 2024\"_\n\n*Suggestions:*\n• Check spelling\n• Try broader search terms\n• Remove filters"
    }
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": "Searched: Documents, Messages, Files"}
    ]
  }
]
```

### First-Time Setup
```json
[
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": ":wave: *Welcome! Let's get started*\n\nThis channel is connected, but there's nothing to show yet."
    }
  },
  {
    "type": "divider"
  },
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Quick Start:*\n1. Connect your first data source\n2. Invite team members\n3. Set up your first automated report\n\n:book: <https://docs.example.com/quickstart|View Quick Start Guide>"
    }
  }
]
```

---

## Compact Fields

**Use for:** Key-value pairs, metadata, side-by-side comparisons

**Why:** The `fields` property in section blocks displays data in a compact two-column layout, perfect for metadata and structured information.

### Basic Key-Value Pairs
```json
[
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Ticket #1234*"
    },
    "fields": [
      {"type": "mrkdwn", "text": "*Status:*\nOpen"},
      {"type": "mrkdwn", "text": "*Priority:*\nHigh"},
      {"type": "mrkdwn", "text": "*Assignee:*\n<@U123>"},
      {"type": "mrkdwn", "text": "*Due:*\nDec 15, 2024"}
    ]
  }
]
```

### Comparison Layout
```json
[
  {
    "type": "header",
    "text": {"type": "plain_text", "text": "Plan Comparison"}
  },
  {
    "type": "section",
    "fields": [
      {"type": "mrkdwn", "text": "*Basic Plan*\n$10/month\n• 5 users\n• 10GB storage\n• Email support"},
      {"type": "mrkdwn", "text": "*Pro Plan*\n$25/month\n• Unlimited users\n• 100GB storage\n• Priority support"}
    ]
  }
]
```

### Metadata Footer Pattern
```json
[
  {
    "type": "section",
    "text": {
      "type": "mrkdwn",
      "text": "*Deploy Summary*\nSuccessfully deployed v2.3.1 to production."
    },
    "fields": [
      {"type": "mrkdwn", "text": "*Environment:*\nProduction"},
      {"type": "mrkdwn", "text": "*Duration:*\n2m 34s"},
      {"type": "mrkdwn", "text": "*Commit:*\n`a1b2c3d`"},
      {"type": "mrkdwn", "text": "*Author:*\n<@U123>"}
    ]
  },
  {
    "type": "context",
    "elements": [
      {"type": "mrkdwn", "text": "Deployed: <!date^1702300800^{date_short} at {time}|Dec 11, 2024> | <https://github.com/org/repo/commit/a1b2c3d|View Commit>"}
    ]
  }
]
```

**Fields guidelines:**
- Maximum 10 fields per section
- Each field displays in a two-column grid
- Keep field content concise (truncates on mobile)
- Use for structured data, not prose

---

## Best Practices

### Do's
- **Always include text fallback** - Shows in notifications and accessibility tools
- **Use dividers sparingly** - One or two per message max
- **Keep headers short** - Plain text only, under 150 chars
- **Use context for metadata** - Timestamps, links, attribution, IDs
- **Match emoji to tone** - Professional vs casual
- **Use `section.fields`** - For compact side-by-side key-value pairs
- **Update messages after actions** - Remove stale buttons, show new state

### Don'ts
- Don't exceed 50 blocks per message
- Don't use headers for body text (they're large and bold)
- Don't skip the text parameter when using blocks
- Don't overuse emoji - 2-3 per message is plenty for professional contexts
- Don't truncate button labels - Keep them short and clear
- Don't use emoji as the only indicator - Always pair with text

### Emoji Rules (per Slack Design Guidelines)

1. **Position**: Place emoji at the **end** of headers, not the beginning
   - Good: `Project Status Update :rocket:`
   - Avoid: `:rocket: Project Status Update`
2. **Frequency**: Use in either header OR body text, not both heavily
3. **Never** use emoji in input labels or as primary button affordances
4. **Always** provide text equivalent for meaning conveyed by emoji
5. **Sparingly**: 2-3 per message maximum for professional contexts

### Emoji Reference

| Purpose | Emoji | Text Equivalent |
|---------|-------|-----------------|
| Success/Complete | `:white_check_mark:` `:tada:` | "Complete", "Done" |
| Warning/Caution | `:warning:` `:large_yellow_circle:` | "At Risk", "Warning" |
| Error/Critical | `:red_circle:` `:rotating_light:` | "Blocked", "Critical" |
| Information | `:information_source:` `:bulb:` | "Note", "Tip" |
| In Progress | `:hourglass:` `:arrows_counterclockwise:` | "In Progress", "Pending" |
| Document/Link | `:memo:` `:link:` `:page_facing_up:` | "Doc", "Link" |
| Metrics Up | `:chart_with_upwards_trend:` `:arrow_up:` | "+X%", "Up" |
| Metrics Down | `:chart_with_downwards_trend:` `:arrow_down:` | "-X%", "Down" |
| Metrics Flat | `:left_right_arrow:` | "No change", "Flat" |
| Calendar/Time | `:calendar:` `:clock3:` | Date/time in text |
| Person/Team | `:bust_in_silhouette:` `:busts_in_silhouette:` | Name or @mention |

---

## Accessibility

Block Kit messages should be accessible to all users, including those using screen readers or with visual impairments.

### Requirements

1. **Alt text for images**: Always include descriptive `alt_text`
   ```json
   {"type": "image", "image_url": "https://...", "alt_text": "Bar chart showing Q4 revenue growth of 15%"}
   ```

2. **Text equivalents**: Never rely solely on emoji or color to convey meaning
   - Bad: `:red_circle:` (what does red mean?)
   - Good: `:red_circle: Blocked` or `Status: Blocked`

3. **Color-blind friendly**: Always pair status colors with text labels
   ```
   :large_green_circle: On Track    (not just the green circle)
   :large_yellow_circle: At Risk    (not just the yellow circle)
   :red_circle: Blocked             (not just the red circle)
   ```

4. **Descriptive links**: Use meaningful link text, not "click here"
   - Bad: `<https://docs.example.com|Click here>`
   - Good: `<https://docs.example.com|View full documentation>`

5. **Logical reading order**: Structure blocks so they make sense when read linearly

### Screen Reader Considerations

- The `text` parameter is read by screen readers in notifications
- `alt_text` on images is read aloud
- `context` blocks are read but may be de-emphasized
- Emoji are read by name (e.g., ":white_check_mark:" reads as "white check mark")