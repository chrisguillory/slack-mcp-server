# Users List Tool - Test Suite

## How to Run These Tests

**Copy and paste this prompt to test the Slack MCP users_list features:**

```
@tests/users_list_test_suite.md

Please run all the tests in this test suite to validate the Slack MCP users_list tool is working correctly.
```

---

## Overview
This test suite validates the new `users_list` tool functionality:
- `filter` parameter for filtering users by type/status
- `fields` parameter for selective field output
- `include_deleted` and `include_bots` parameters for inclusion control
- Proper cursor handling and pagination
- Error handling and edge cases

## Test Categories

### 1. Field Selection Tests
Tests the `fields` parameter functionality and token optimization.

#### Test 1.1: Default fields (minimal output)
**Prompt:** "List all users in the workspace"
**Tool call:** `fields: "id,name,real_name,status"` (default behavior)
**Expected:** Only `id`, `name`, `real_name`, and `status` fields
**Verification:** CSV has only 4 columns

#### Test 1.2: Explicit minimal fields
**Prompt:** "List all users with just their IDs and names"
**Tool call:** `fields: "id,name"`
**Expected:** Only ID and Name columns
**Verification:** Only 2 columns in output

#### Test 1.3: Request specific fields
**Prompt:** "Show me all users with their names, emails, and admin status"
**Tool call:** `fields: "name,email,is_admin"`
**Expected:** Only Name, Email, and IsAdmin columns
**Verification:** Only 3 columns: Name, Email, IsAdmin

#### Test 1.4: All fields
**Prompt:** "List all users with complete details including title, phone, and timezone"
**Tool call:** `fields: "all"`
**Expected:** All fields returned: ID, Name, RealName, Email, Status, IsBot, IsAdmin, TimeZone, Title, Phone
**Verification:** All 10 fields present

---

### 2. User Filtering Tests
Tests the `filter` parameter for filtering users by type/status.

#### Test 2.1: Active users only
**Prompt:** "Show me only active users in the workspace"
**Tool call:** `filter: "active"` + `include_deleted: false`
**Expected:** Only active (non-deleted) users
**Verification:** No users with deleted status

#### Test 2.2: Bot users only
**Prompt:** "List all bot users"
**Tool call:** `filter: "bots"`
**Expected:** Only bot users returned
**Verification:** All users have IsBot: true

#### Test 2.3: Human users only
**Prompt:** "Show me human users only, no bots"
**Tool call:** `filter: "humans"`
**Expected:** Only non-bot users
**Verification:** All users have IsBot: false

#### Test 2.4: Admin users
**Prompt:** "List all admin and owner users"
**Tool call:** `filter: "admins"`
**Expected:** Only users with admin/owner privileges
**Verification:** All users have IsAdmin: true

#### Test 2.5: Deleted users
**Prompt:** "Show deleted/deactivated users"
**Tool call:** `filter: "deleted"` + `include_deleted: true`
**Expected:** Only deleted users
**Verification:** All users have Status: deleted

---

### 3. Search/Query Tests
Tests the `query` parameter for searching users by name.

#### Test 3.1: Search by username
**Prompt:** "Find all users named Chris"
**Tool call:** `query: "chris"`
**Expected:** All users with "chris" in their username (case-insensitive)
**Verification:** All returned users have "chris" in Name field

#### Test 3.2: Search by real name
**Prompt:** "Find users with Smith in their real name"
**Tool call:** `query: "smith"`
**Expected:** All users with "smith" in their real name
**Verification:** All returned users have "smith" in RealName field

#### Test 3.3: Case-insensitive search
**Prompt:** "Search for users named JOHN"
**Tool call:** `query: "JOHN"`
**Expected:** Matches john, John, JOHN in any name field
**Verification:** Case-insensitive matching works

#### Test 3.4: Partial name search
**Prompt:** "Find users whose name contains 'bot'"
**Tool call:** `query: "bot"`
**Expected:** All bot users and users with "bot" in their name
**Verification:** Partial matching works

#### Test 3.5: Search with filter combination
**Prompt:** "Find active users named Chris"
**Tool call:** `query: "chris"` + `filter: "active"`
**Expected:** Only active users with "chris" in their name
**Verification:** Search and filter work together

#### Test 3.6: Search with field selection
**Prompt:** "Find users named Alex and show just their names and emails"
**Tool call:** `query: "alex"` + `fields: "name,email"`
**Expected:** Users with "alex" in name, only Name and Email columns
**Verification:** Search works with field selection

---

### 4. Inclusion Control Tests
Tests the `include_deleted` and `include_bots` parameters.

#### Test 4.1: Exclude deleted users (default)
**Prompt:** "List active users without deleted accounts"
**Tool call:** `include_deleted: false` (default)
**Expected:** No deleted users in results
**Verification:** No users with Status: deleted

#### Test 4.2: Include deleted users
**Prompt:** "List all users including deleted ones"
**Tool call:** `include_deleted: true`
**Expected:** Both active and deleted users
**Verification:** Mix of active and deleted status values

#### Test 4.3: Exclude bot users
**Prompt:** "List users but exclude bots"
**Tool call:** `include_bots: false`
**Expected:** No bot users in results
**Verification:** No users with IsBot: true

#### Test 4.4: Include bot users (default)
**Prompt:** "List all users including bots"
**Tool call:** `include_bots: true` (default)
**Expected:** Both human and bot users
**Verification:** Mix of IsBot: true and false

---

### 5. Combined Parameters Tests
Tests how multiple parameters work together.

#### Test 5.1: Minimal fields + active filter
**Prompt:** "Give me just the names of active human users"
**Tool call:** `fields: "name"` + `filter: "humans"` + `include_deleted: false`
**Expected:** Single column CSV with just names, only active human users
**Verification:** Single column, no bots, no deleted users

#### Test 5.2: Specific fields + admin filter
**Prompt:** "Show admin users with their names, emails, and titles"
**Tool call:** `fields: "name,email,title"` + `filter: "admins"`
**Expected:** Three columns for admin users only
**Verification:** Three columns, all users are admins

#### Test 5.3: All fields + bots filter
**Prompt:** "Show complete details for all bot users"
**Tool call:** `fields: "all"` + `filter: "bots"`
**Expected:** All fields for bot users only
**Verification:** All 10 fields, all users have IsBot: true

---

### 6. Pagination & Cursor Tests
Tests that pagination works correctly with the users list.

#### Test 6.1: Basic pagination
**Prompt:** "List first 10 users"
**Tool call:** `limit: 10`
**Expected:** 10 rows of data, cursor in metadata if more users exist
**Verification:** 10 rows, cursor in metadata (not in CSV)

#### Test 6.2: Pagination with cursor
**Prompt:** "Get the next page of users after cursor XYZ"
**Tool call:** `limit: 10` + `cursor: "XYZ"`
**Expected:** Next set of results, new cursor if more pages exist
**Verification:** Next page retrieved, consistent field selection

#### Test 6.3: Large limit (default)
**Prompt:** "List all users up to 1000"
**Tool call:** `limit: 1000` (default)
**Expected:** Up to 1000 users returned
**Verification:** All available users up to 1000

---

### 7. Edge Cases & Error Handling Tests
Tests error handling and edge cases.

#### Test 7.1: Invalid field names
**Prompt:** "List users with their names and passwords"
**Tool call:** `fields: "name,password"` (password is not a valid field)
**Expected:** Should handle gracefully, ignore invalid field
**Verification:** Invalid field ignored, only name returned

#### Test 7.2: Empty results with filtering
**Prompt:** "Show users with filter that returns no results"
**Tool call:** `filter: "deleted"` + `include_deleted: false` (contradictory)
**Expected:** Empty result set handled gracefully
**Verification:** Empty CSV with headers only

#### Test 7.3: Invalid filter value
**Prompt:** "List users with invalid filter"
**Tool call:** `filter: "invalid_filter"`
**Expected:** Defaults to "all" filter
**Verification:** All users returned (same as filter: "all")

---

### 8. Performance & Token Usage Tests
Tests performance improvements and token reduction.

#### Test 8.1: Measure token reduction
**Before (all fields):** `fields: "all"` with 50 users
**After (minimal):** `fields: "name"` with 50 users
**Expected:** 80-90% reduction in output size
**Verification:** Compare response sizes

#### Test 8.2: Large dataset with minimal fields
**Prompt:** "List all user names only"
**Tool call:** `fields: "name"` + `limit: 1000`
**Expected:** Fast response with minimal data
**Verification:** Efficient handling of large user lists

---

## Test Prompts

### Quick Validation (Run these first)
**Test 1.1: Default fields**
```
List all users in the workspace
```

**Test 2.2: Bot users**
```
List all bot users
```

**Test 2.3: Human users**
```
Show me human users only, no bots
```

**Test 2.4: Admin users**
```
List all admin and owner users
```

**Test 3.1: Search for users**
```
Find all users named Chris
```

---

## Expected Results Summary

| Test Category | Expected Outcome | Token Reduction |
|---------------|------------------|-----------------|
| Field Selection | Minimal fields by default | 60-80% |
| User Filtering | Correct filtering by type | 20-80% |
| Search/Query | Case-insensitive name search | Variable |
| Inclusion Control | Proper include/exclude logic | Variable |
| Combined Features | All features work together | 70-90% |
| Pagination | Cursor handling works | N/A |
| Error Handling | Graceful degradation | N/A |
| Performance | Fast response times | 60-90% |

---

## Verification Checklist

After running all tests, verify:

- [ ] Default behavior returns minimal fields (id, name, real_name, status)
- [ ] `query` parameter searches users by name (case-insensitive)
- [ ] Search works across username, real name, and display name fields
- [ ] `fields` parameter correctly filters columns
- [ ] `fields: "all"` returns all available fields
- [ ] `filter` parameter correctly filters user types
- [ ] `include_deleted` controls deleted user inclusion
- [ ] `include_bots` controls bot user inclusion
- [ ] Search and filter parameters work together correctly
- [ ] Cursor appears in metadata, not CSV data
- [ ] Invalid fields handled gracefully
- [ ] Token usage reduced by 60-80% for typical queries
- [ ] Pagination works correctly
- [ ] Default limit is 1000 users

---

## Comparison with channels_list

The `users_list` tool follows similar patterns to `channels_list`:

| Feature | channels_list | users_list |
|---------|--------------|------------|
| Default limit | 1000 | 1000 |
| Field selection | ✓ | ✓ |
| Search/Query | ✗ | ✓ (by name) |
| Pagination | ✓ | ✓ |
| Type filtering | channel_types | filter |
| Member filtering | min_members | N/A |
| Inclusion control | N/A | include_deleted, include_bots |
| Default fields | id,name | id,name,real_name,status |

---

## Notes for Future Iteration

1. **Add search functionality** - Search users by name or email
2. **Add sorting options** - Sort by name, join date, last active
3. **Add status filtering** - Filter by online/offline status
4. **Add team filtering** - For Enterprise Grid workspaces
5. **Add custom field support** - For workspace-specific user fields

---

## Troubleshooting

### Common Issues
- **No users returned:** Check if users cache is populated
- **Missing fields:** Verify field names match exactly
- **Unexpected filtering:** Check filter parameter spelling
- **Pagination issues:** Ensure cursor is properly passed

### Debug Tips
- **Test with small limits first:** Use `limit: 5` for testing
- **Verify field names:** Check exact field names in documentation
- **Test filters individually:** Test each filter type separately
- **Check cache status:** Ensure `.users_cache.json` exists and is populated