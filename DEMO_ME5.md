# ME5 Demo — Frontend: Run & Observe

This walkthrough covers every testable criterion for ME5. It assumes the full stack is running (`docker compose up --build`) and that ME4 setup has already been completed (at least one workflow with an LLM node, one eval suite, and at least two test cases with different grader types).

Prerequisites:
- A workflow that is saved and has at least one LLM node.
- An eval suite with at least two test cases:
  - Test case A: one String Match grader (scope: workflow) and one LLM Judge grader (scope: node).
  - Test case B: one Checklist grader (scope: node) with 3+ criteria and pass_threshold 0.6.
- Both test cases have node mocks set for any side-effectful nodes.

---

## 1. Trigger a run and watch live polling

1. Open the eval suite in the browser.
2. Click **Run Suite**.
3. Observe: the browser navigates to `/eval-runs/<id>`.
4. Verify the status badge shows **running** (amber).
5. Verify the text "Polling every 2s…" is visible and animating.
6. Every two seconds the page re-fetches without a manual refresh.

Expected: the run detail page updates automatically. No manual refresh needed.

---

## 2. Run completes — summary counts

When the run reaches **completed** (green badge) or **failed** (red badge):

1. Verify the polling indicator disappears.
2. Verify the four summary tiles (Total / Passed / Failed / Errors) show non-zero values matching the test case results.
3. Verify Started time and duration (e.g., "12s") are shown beneath the tiles.

Expected: all four counts are present and sum to the total test case count.

---

## 3. Expandable test case rows

1. Verify each test case appears as a collapsed row with:
   - Test case name.
   - A workflow run status chip (e.g., **succeeded** or **failed**).
   - A **View Run →** link.
   - An overall verdict: **✓ passed** (green) or **✗ failed** (red).
2. Click a row to expand it.
3. Verify the row expands to show grader details.
4. Click again to collapse.

Expected: rows toggle open and closed without page navigation.

---

## 4. GraderResultRow — per-grader verdicts

Inside an expanded test case row:

1. Verify each grader shows:
   - A verdict icon: ✓ (green), ✗ (red), or ! (amber for error).
   - The grader name.
   - A type chip: "String Match", "LLM Judge", "Numeric", "JSON Schema", or "Checklist".
2. For the LLM Judge grader, verify the explanation text appears beneath the verdict line (e.g., "The response acknowledges the issue and offers resolution").
3. Click **show value** on any grader row.
4. Verify a pre-formatted block appears showing the actual value that was inspected.
5. Click **hide value** — block collapses.

Expected: the actual_value toggle works; explanation text is visible; type chips are correct.

---

## 5. ChecklistResultDetail — per-criterion breakdown

Expand test case B (the one with the Checklist grader):

1. Verify the Checklist grader row shows:
   - A score percentage (e.g., "80%").
   - The overall verdict (pass or fail based on pass_threshold).
2. Verify a per-criterion table appears below the verdict line showing:
   - Each criterion text.
   - A ✓ (green) or ✗ (red) per criterion.
   - The judge's explanation for each criterion.
3. Verify the summary line at the bottom reads e.g., "3 of 4 criteria met — 75%".

Expected: the checklist detail table is visible inside the expanded row without any additional click.

---

## 6. Workflow run failure surfacing

To test this, temporarily break a node in the workflow (e.g., point an HTTP node at an unreachable URL) and trigger another run:

1. Verify the failing test case row has a red border.
2. Expand the row.
3. Verify a red banner appears: "Workflow run failed — graders that depended on node output could not be evaluated."
4. Verify graders that could not execute show a verdict of **!** (amber error).
5. Verify the **View Run →** link opens the workflow run detail page for that underlying run.

Expected: failure is surfaced clearly; the link to the underlying run is present and navigates correctly.

---

## 7. View Run link navigation

1. In any expanded test case row, click **View Run →**.
2. Verify navigation goes to `/runs/<workflow_run_id>` — the existing workflow run detail page.
3. Verify the node status colours on that page match what the eval captured (nodes that were mocked show their mock output).
4. Use the browser back button to return to the eval run detail page.

Expected: navigation works; browser back returns to eval run detail.

---

## 8. Run History panel — new run appears

1. Navigate back to the eval suite detail page.
2. Expand the **Run History** accordion (click it once).
3. Verify the run just completed appears at the top of the list with:
   - The first 8 characters of the run ID.
   - The started-at timestamp.
   - The duration (e.g., "14s").
   - The pass summary (e.g., "2/2 passed").
   - The status badge (**completed** or **failed**).
4. Click the run row — verify navigation to `/eval-runs/<id>`.

Expected: the new run is listed; duration is shown; clicking navigates to the detail page.

---

## 9. Multiple independent runs

1. Trigger a second run without changing any test cases.
2. In the Run History panel, verify two independent rows appear.
3. Open each run — verify the results are independent (same counts if the workflow and mocks are stable).
4. Edit one test case (change a grader's expected value) and trigger a third run.
5. Open the first run — verify its results are unchanged (snapshots are preserved at run time).

Expected: historical results do not change when test cases are edited after the run.

---

## 10. Navigate away mid-poll — no console warnings

1. Trigger a run.
2. Immediately navigate away to the workflow list page while the run is in "running" state.
3. Open the browser dev tools console.
4. Verify no "Can't perform a React state update on an unmounted component" warning appears.

Expected: the `alive` guard in the polling effect prevents post-unmount state updates.

---

## Cleanup

After the demo, you can delete the test suites created during setup:

```
DELETE /v1/eval-suites/<suite_id>
```

Or use the **Delete** button on the suite detail page.
