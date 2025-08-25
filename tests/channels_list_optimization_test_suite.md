# Channels List Token Optimization - Test Suite

## How to Run These Tests

**Copy and paste this prompt to test the Slack MCP channels_list optimization features:**

```
@tests/channels_list_optimization_test_suite.md

Please run all the tests in this test suite to validate the Slack MCP channels_list optimization features are working correctly.
```

---

## Overview
This test suite validates the new token optimization features for the `channels_list` tool:
- `fields` parameter for selective field output
- `min_members` parameter for filtering low-activity channels
- Proper cursor handling and pagination
- Error handling and edge cases

## Test Categories

### 1. Field Selection Tests
Tests the `fields` parameter functionality and token optimization.

#### Test 1.1: Default fields (minimal output)
**Prompt:** "List all public channels"
**Tool call:** `fields: "id,name"` (default behavior)
**Expected:** Only `id` and `name` fields, ~80% token reduction
**Verification:** CSV has only 2 columns, no topic/purpose columns

#### Test 1.2: Explicit minimal fields
**Prompt:** "List all public channels with just their IDs and names"
**Tool call:** `fields: "id,name"`
**Expected:** Only ID and Name columns, no topic/purpose
**Verification:** Verify no topic/purpose columns

#### Test 1.3: Request specific fields
**Prompt:** "Show me all public channels with their names and member counts"
**Tool call:** `fields: "name,member_count"`
**Expected:** Only Name and MemberCount columns, no ID, topic, or purpose
**Verification:** Only 2 columns: Name, MemberCount

#### Test 1.4: All fields (backward compatibility)
**Prompt:** "List all public channels with complete details including topics and purposes"
**Tool call:** `fields: "all"`
**Expected:** All fields returned: ID, Name, Topic, Purpose, MemberCount
**Verification:** Same output as before optimization

---

### 2. Search/Query Tests
Tests the `query` parameter for searching channels by name, topic, and purpose.

#### Test 2.1: Search by channel name
**Prompt:** "Find all channels with 'acq' in the name"
**Tool call:** `query: "acq"`
**Expected:** All channels with "acq" in their name (case-insensitive)
**Verification:** All returned channels have "acq" in Name field

#### Test 2.2: Search by topic
**Prompt:** "Find channels with 'marketing' in their topic"
**Tool call:** `query: "marketing"`
**Expected:** All channels with "marketing" in their topic
**Verification:** Channels with "marketing" in Topic field are included

#### Test 2.3: Search by purpose
**Prompt:** "Find channels with 'support' in their purpose"
**Tool call:** `query: "support"`
**Expected:** All channels with "support" in their purpose
**Verification:** Channels with "support" in Purpose field are included

#### Test 2.4: Case-insensitive search
**Prompt:** "Search for channels with 'DEV'"
**Tool call:** `query: "DEV"`
**Expected:** Matches dev, Dev, DEV in name/topic/purpose
**Verification:** Case-insensitive matching works

#### Test 2.5: Partial name search
**Prompt:** "Find channels containing 'test'"
**Tool call:** `query: "test"`
**Expected:** All test channels and channels with "test" anywhere
**Verification:** Partial matching works

#### Test 2.6: Search with type filter
**Prompt:** "Find public channels with 'project' in the name"
**Tool call:** `query: "project"` + `channel_types: "public_channel"`
**Expected:** Only public channels with "project" in name/topic/purpose
**Verification:** Search and type filter work together

#### Test 2.7: Search with field selection
**Prompt:** "Find channels with 'sales' and show just names and member counts"
**Tool call:** `query: "sales"` + `fields: "name,member_count"`
**Expected:** Channels with "sales", only Name and MemberCount columns
**Verification:** Search works with field selection

---

### 3. Member Count Filtering Tests
Tests the `min_members` parameter for filtering channels by activity level.

#### Test 3.1: Filter by minimum members
**Prompt:** "Show me active public channels with at least 10 members"
**Tool call:** `fields: "name,member_count"` + `min_members: 10`
**Expected:** Only channels with memberCount >= 10, significantly fewer results
**Verification:** Only channels with 10+ members, ~61% reduction

#### Test 3.2: Find popular channels
**Prompt:** "List the most popular channels with over 50 members, sorted by popularity"
**Tool call:** `fields: "name,member_count"` + `min_members: 50` + `sort: "popularity"`
**Expected:** Only high-member channels, sorted by member count descending
**Verification:** Only channels with 50+ members, ~93% reduction, sorted

#### Test 3.3: Exclude empty/test channels
**Prompt:** "Show me real active channels, not test or abandoned ones"
**Tool call:** `fields: "name,member_count"` + `min_members: 3`
**Expected:** No channels with 0-2 members, cleaner results
**Verification:** No channels with 0-2 members, ~34% reduction

---

### 4. Combined Parameters Tests
Tests how multiple optimization features work together.

#### Test 4.1: Minimal output + filtering
**Prompt:** "Give me just the names of active channels with at least 5 members"
**Tool call:** `fields: "name"` + `min_members: 5`
**Expected:** Single column CSV with just names, only channels with 5+ members
**Verification:** Single column, only channels with 5+ members, ~45% reduction

#### Test 4.2: Specific fields + filtering + sorting
**Prompt:** "Show channel names and member counts for channels with 10+ members, sorted by popularity"
**Tool call:** `fields: "name,member_count"` + `min_members: 10` + `sort: "popularity"`
**Expected:** Two columns: Name, MemberCount, filtered to 10+ members, sorted
**Verification:** Two columns, filtered to 10+ members, sorted, ~61% reduction

#### Test 4.3: Search + filtering + field selection
**Prompt:** "Find channels with 'dev' in the name, with 5+ members, show name and member count"
**Tool call:** `query: "dev"` + `min_members: 5` + `fields: "name,member_count"`
**Expected:** Only channels with "dev" and 5+ members, two columns
**Verification:** Combined search, filter, and field selection work together

---

### 5. Pagination & Cursor Tests
Tests that pagination works correctly with optimization features.

#### Test 5.1: Check cursor not in CSV
**Prompt:** "List first 5 public channels"
**Tool call:** `fields: "id,name"` + `limit: 5`
**Expected:** 5 rows of data, cursor NOT in last CSV row, cursor in metadata
**Verification:** 5 rows, cursor in metadata only, not in CSV data

#### Test 5.2: Pagination with field selection
**Prompt:** "Get the next page of channels after cursor XYZ"
**Tool call:** `fields: "id,name"` + `limit: 5` + `cursor: "XYZ"`
**Expected:** Next set of results, still only 2 columns, new cursor provided
**Verification:** Next page retrieved, field optimization maintained

---

### 6. Edge Cases & Error Handling Tests
Tests error handling and edge cases.

#### Test 6.1: Invalid field names
**Prompt:** "List channels with their names and descriptions"
**Tool call:** `fields: "name,description"` (description is not a valid field)
**Expected:** Should handle gracefully, ignore invalid field, return valid fields
**Verification:** Invalid field ignored, only valid fields returned

#### Test 6.2: Empty results with filtering
**Prompt:** "Show channels with at least 1000 members"
**Tool call:** `fields: "name,member_count"` + `min_members: 1000`
**Expected:** Likely empty or very few results, should handle empty CSV gracefully
**Verification:** Single result returned without errors

#### Test 6.3: MPIMs and IMs
**Prompt:** "List all direct messages with just names"
**Tool call:** `channel_types: "im"` + `fields: "name"`
**Expected:** Only DM names, verify field selection works for all channel types
**Verification:** Field selection works correctly with different channel types

---

### 7. Performance & Token Usage Tests
Tests performance improvements and token reduction.

#### Test 7.1: Measure token reduction
**Before optimization:** `fields: "all"` with 10 channels
**After optimization:** `fields: "name"` with 10 channels
**Expected:** 70-80% reduction for typical queries
**Verification:** Compare response sizes/tokens

#### Test 7.2: Large result set with filtering
**Prompt:** "Show all channels with 5+ members, just names"
**Tool call:** `fields: "name"` + `min_members: 5` + `limit: 1000`
**Expected:** Should be much faster/smaller than full output
**Verification:** Efficient handling of large datasets with filtering

---

## Test Prompts

### Quick Validation (Run these first)
**Test 1.1: Default fields**
```
List all public channels
```

**Test 1.4: All fields (backward compatibility)**
```
List all public channels with complete details including topics and purposes
```

**Test 2.1: Search channels**
```
Find all channels with 'acq' in the name
```

**Test 3.1: Member filtering**
```
Show me active public channels with at least 10 members
```

---

## Expected Results Summary

| Test Category | Expected Outcome | Token Reduction |
|---------------|------------------|-----------------|
| Field Selection | Minimal fields by default | 70-80% |
| Search/Query | Case-insensitive search in name/topic/purpose | Variable |
| Member Filtering | Quality data improvement | 30-90% |
| Combined Features | All features work together | 60-90% |
| Pagination | Cursor handling works | N/A |
| Error Handling | Graceful degradation | N/A |
| Performance | Fast response times | 70-90% |

---

## Verification Checklist

After running all tests, verify:

- [ ] Default behavior returns minimal fields (id, name)
- [ ] `query` parameter searches channels by name, topic, and purpose (case-insensitive)
- [ ] Search works with partial matches
- [ ] `fields` parameter correctly filters columns
- [ ] `fields: "all"` maintains backward compatibility
- [ ] `min_members` correctly filters channels
- [ ] Search and filters work together correctly
- [ ] Cursor no longer appears in CSV data
- [ ] Invalid fields handled gracefully
- [ ] Token usage reduced by 70-80% for typical queries
- [ ] All channel types work with new parameters
- [ ] Pagination still works correctly
- [ ] Sorting works with filtered fields

---

## Notes for Future Iteration

1. **Add new test cases** as features evolve
2. **Performance benchmarks** for different dataset sizes
3. **Integration tests** with other MCP tools
4. **Automated testing** scripts for CI/CD
5. **User acceptance testing** scenarios

---

## Troubleshooting

### Common Issues
- **Field not found errors:** Check field name spelling
- **Empty results:** Verify `min_members` threshold isn't too high
- **Pagination issues:** Ensure cursor is properly encoded

### Debug Tips
- **Test with minimal data first:** Use `limit: 5` to test with small datasets
- **Verify field names:** Check that field names match exactly (e.g., `member_count`, not `memberCount`)
- **Test edge cases:** Try invalid field names to verify error handling
- **Compare outputs:** Run the same test with different `fields` values to see the difference
