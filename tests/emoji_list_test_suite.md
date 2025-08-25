# Emoji List Tool - Test Suite

## How to Run These Tests

**Copy and paste this prompt to test the Slack MCP emoji_list features:**

```
@tests/emoji_list_test_suite.md

Please run all the tests in this test suite to validate the Slack MCP emoji_list tool is working correctly.
```

---

## Overview
This test suite validates the new `emoji_list` tool functionality:
- Listing all available emojis/reactions in the workspace
- `type` parameter for filtering by emoji type (all, custom, unicode)
- `query` parameter for searching emojis by name
- `limit` parameter for controlling result size (default: 1000)
- Proper cursor handling and pagination
- Cached emoji data retrieval

## Test Categories

### 1. Basic Listing Tests
Tests the basic emoji listing functionality.

#### Test 1.1: List all emojis with default settings
**Prompt:** "Show me all available emoji reactions"
**Tool call:** `emoji_list` with no parameters
**Expected:** 
- CSV with headers: Name, URL, IsCustom, Aliases, TeamID, UserID
- Up to 1000 emojis returned
- Mix of custom and unicode emojis
**Verification:** 
- Check metadata shows total count
- Check CSV has proper headers
- Both custom (IsCustom=true) and unicode (IsCustom=false) emojis present

#### Test 1.2: List with smaller limit
**Prompt:** "Show me the first 10 emojis"
**Tool call:** `emoji_list` with `limit: 10`
**Expected:** 
- Maximum 10 emoji entries
- Cursor provided if more emojis exist
**Verification:** 
- Count data rows ≤ 10
- Check for "# Next cursor:" in metadata

### 2. Type Filtering Tests
Tests filtering emojis by type.

#### Test 2.1: Custom emojis only
**Prompt:** "List all custom workspace emojis"
**Tool call:** `emoji_list` with `type: "custom"`
**Expected:** 
- Only custom emojis (IsCustom=true)
- TeamID populated for custom emojis
**Verification:** 
- All rows have IsCustom=true
- TeamID field is not empty

#### Test 2.2: Unicode emojis only
**Prompt:** "Show me standard unicode emojis"
**Tool call:** `emoji_list` with `type: "unicode"`
**Expected:** 
- Only unicode/standard emojis (IsCustom=false)
- Common emojis like thumbsup, heart, smile
**Verification:** 
- All rows have IsCustom=false
- TeamID field is empty
- Contains common emojis (thumbsup, heart, etc.)

### 3. Search Tests
Tests the query/search functionality.

#### Test 3.1: Search for specific emoji
**Prompt:** "Find emojis with 'thumb' in the name"
**Tool call:** `emoji_list` with `query: "thumb"`
**Expected:** 
- Results containing "thumb" in name
- Should find thumbsup, thumbsdown
**Verification:** 
- All results contain "thumb" (case-insensitive)
- Includes thumbsup and thumbsdown

#### Test 3.2: Search with type filter
**Prompt:** "Find custom emojis with 'fire' in the name"
**Tool call:** `emoji_list` with `query: "fire"` and `type: "custom"`
**Expected:** 
- Only custom emojis with "fire" in name
**Verification:** 
- All results have IsCustom=true
- All names contain "fire"

#### Test 3.3: Search with no results
**Prompt:** "Find emojis named 'xyznonexistent'"
**Tool call:** `emoji_list` with `query: "xyznonexistent"`
**Expected:** 
- Empty result set (only headers)
- Metadata shows "# Total emojis: 0"
**Verification:** 
- No data rows (only headers)
- Total count is 0

### 4. Pagination Tests
Tests cursor-based pagination.

#### Test 4.1: First page with cursor
**Prompt:** "Show me emojis with pagination (5 per page)"
**Tool call:** `emoji_list` with `limit: 5`
**Expected:** 
- 5 emojis returned
- Cursor provided for next page
**Verification:** 
- Exactly 5 data rows
- Cursor present in metadata

#### Test 4.2: Second page using cursor
**Prompt:** "Get the next page of emojis using cursor: [cursor_from_previous]"
**Tool call:** `emoji_list` with `limit: 5` and `cursor: "[cursor_value]"`
**Expected:** 
- Different set of 5 emojis
- New cursor or "(none - last page)"
**Verification:** 
- 5 or fewer emojis
- Different emojis than first page

### 5. Combined Parameter Tests
Tests using multiple parameters together.

#### Test 5.1: Search custom emojis with limit
**Prompt:** "Find the first 3 custom emojis with 'a' in the name"
**Tool call:** `emoji_list` with `query: "a"`, `type: "custom"`, `limit: 3`
**Expected:** 
- Maximum 3 custom emojis
- All contain 'a' in name
**Verification:** 
- ≤ 3 rows
- All have IsCustom=true
- All names contain 'a'

### 6. Edge Cases and Error Handling

#### Test 6.1: Invalid type parameter
**Prompt:** "List emojis of type 'invalid'"
**Tool call:** `emoji_list` with `type: "invalid"`
**Expected:** 
- Should default to "all" behavior
- Return all emoji types
**Verification:** 
- Returns results (doesn't error)
- Mix of custom and unicode emojis

#### Test 6.2: Limit boundary testing
**Prompt:** "List emojis with limit of 0"
**Tool call:** `emoji_list` with `limit: 0`
**Expected:** 
- Should use default limit (1000)
**Verification:** 
- Returns emojis (not empty)
- Uses default behavior

#### Test 6.3: Limit exceeding maximum
**Prompt:** "List 5000 emojis"
**Tool call:** `emoji_list` with `limit: 5000`
**Expected:** 
- Should cap at 1000
**Verification:** 
- Maximum 1000 emojis returned
- Check metadata mentions capping

### 7. Output Format Validation

#### Test 7.1: CSV structure validation
**Prompt:** "List 5 emojis and verify CSV format"
**Tool call:** `emoji_list` with `limit: 5`
**Expected:** 
- Proper CSV with headers
- Metadata lines start with #
- Blank line between metadata and CSV
**Verification:** 
- First lines start with # (metadata)
- Headers: Name,URL,IsCustom,Aliases,TeamID,UserID
- Data rows have 6 fields

#### Test 7.2: Aliases field formatting
**Prompt:** "Show emojis and check alias formatting"
**Tool call:** `emoji_list` with `limit: 10`
**Expected:** 
- Aliases separated by pipe (|) character
- Empty string if no aliases
**Verification:** 
- Aliases field either empty or contains |
- Multiple aliases use | separator

## Expected Behaviors

### Metadata Format
All responses should include metadata at the beginning:
```
# Total emojis: [number]
# Returned in this page: [number]
# Next cursor: [cursor or "(none - last page)"]
```

### CSV Headers
Standard headers for all responses:
```
Name,URL,IsCustom,Aliases,TeamID,UserID
```

### Common Unicode Emojis
The tool should include common unicode emojis like:
- thumbsup (👍)
- thumbsdown (👎)
- heart (❤️)
- smile (😊)
- fire (🔥)
- rocket (🚀)
- tada (🎉)
- white_check_mark (✅)

### Performance Notes
- The emoji list is cached on server startup
- Subsequent calls should be fast (using cache)
- Cache file: `.emojis_cache.json`

## Success Criteria
- All basic listing tests pass
- Type filtering works correctly
- Search functionality is case-insensitive
- Pagination works with cursors
- Combined parameters work together
- Edge cases handled gracefully
- CSV format is valid and consistent