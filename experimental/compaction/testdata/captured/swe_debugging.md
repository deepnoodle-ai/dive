Your conversation history was compacted to free up context. A previous instance of you was working on this task and left the handoff notes below. Treat them as an accurate record of what happened and continue the work seamlessly.

## Task Overview
Fix a failing CI test in the `payments` package and address a related finance report about off-by-a-cent totals in multi-line orders.

**Constraints:**
- Hard constraint: do NOT modify the test `TestCheckout_AppliesDiscount` or its expected values; fix only production code.
- Do not weaken any existing tests.
- Add a regression test for the discount bug (multi-item case preferred).
- Investigate and fix the cent-level drift in multi-line orders if it's a real bug.

**Success criteria:**
1. `TestCheckout_AppliesDiscount` passes.
2. New regression test added (multi-item discount).
3. Finance's off-by-a-cent issue identified and fixed.
4. Full test suite passes with no regressions.

---

## Current State
**Task is COMPLETE.** All changes have been implemented and verified.

### Files Modified:
1. **`payments/checkout.go`** (line 61)
   - Changed: `return Receipt{Total: order.Subtotal, ...}` → `return Receipt{Total: discounted, ...}`
   - Reason: discounted value was computed but never used in the receipt.

2. **`payments/checkout_test.go`** (added after `TestCheckout_NoDiscount`)
   - Added: `TestCheckout_AppliesDiscount_MultiItem` — tests 25% off a $200 two-item order → $150.
   - Purpose: regression test stronger than the single-item existing test.

3. **`payments/tax.go`** (complete function rewrite)
   - Changed: `ApplyTax()` from per-line rounding + sum to single subtotal rounding.
   - Old behavior: taxed each line independently, rounded each, summed → accumulated rounding drift.
   - New behavior: sum pre-tax lines, tax the subtotal once, round once.
   - Example: three $3.33 lines at 8% tax now correctly total $10.79 instead of $10.80.

4. **`payments/tax_test.go`** (created)
   - Added: `TestApplyTax_NoPerLineDrift` — reproduces the three-$3.33-line case that drifted $0.01.
   - Verifies the fix prevents per-line rounding accumulation.

### Test Results:
- ✅ All original tests still pass (no regressions).
- ✅ New discount regression test passes.
- ✅ New tax drift test passes.
- ✅ Full suite (`go test ./...`): 4 packages, all green (payments, orders, api, internal/config).

---

## Important Discoveries

### Root Cause #1: Discount Discarded
- **File:** `payments/checkout.go`, `Checkout()` function.
- **The bug:** Line 55 computed `discounted := ApplyDiscount(order.Subtotal, order.DiscountPct)`, line 56 threw it away with `_ = discounted`, line 61 returned the full `order.Subtotal`.
- **Why it happened:** Dead code (likely accidental during refactoring).
- **Verification:** `ApplyDiscount()` in `discount.go` was already correct (10% pct returns 90% of subtotal via `MulPercent`).

### Root Cause #2: Per-Line Tax Rounding Drift
- **File:** `payments/tax.go`, `ApplyTax()` function.
- **The bug:** Taxed each line independently with `ln.Amount.MulPercent(100 + TaxRatePct)`, rounded each result, appended to output, summed into total. Each line's fractional cent was rounded separately, causing errors to accumulate.
- **Finance report correlation:** "off by a cent or two" on multi-line orders matches this pattern.
- **Example:** Three $3.33 items (total $9.99):
  - Per-line: 3.33 × 1.08 = 3.5964 → rounds to 3.60 per line × 3 = $10.80.
  - Correct (subtotal rounding): 9.99 × 1.08 = 10.7892 → rounds to $10.79.
  - **Drift: +$0.01 per three-item order.**
- **Why it matters:** Finance expected `receipt.Total` to match `subtotal.MulPercent(100 + TaxRatePct)`, which is the correct accounting method (tax the order, not each line).

### Design Decision: Line Amounts Remain Pre-Tax
- Kept `Line.Amount` pre-tax (not modified by `ApplyTax`).
- Only `Receipt.Total` is taxed.
- Rationale: per-line tax display wasn't what finance flagged; the total is the "single source of truth" per the `Receipt` docstring. Modifying lines would be unnecessary complexity.

### Approach Tried:
- Initially considered whether `MulPercent()` itself was buggy → confirmed it uses correct round-half-up logic.
- Narrowed focus to `ApplyTax()` as the only place doing per-line percentage math.

---

## Next Steps
**None.** The task is complete:
- ✅ Original test passes.
- ✅ Regression test added (multi-item discount).
- ✅ Tax drift fixed and tested.
- ✅ Full suite green.

---

## Context to Preserve
- **Code style:** Tests in this repo use direct `if got != want` assertions without table-driven patterns; match this style for consistency.
- **Money type:** Stored as `int64` (cents), not float, to avoid floating-point errors. All math uses `MulPercent()` for percentage operations.
- **Receipt design:** `Receipt.Total` is the single source of truth for customer charge; it is the only field that should reflect tax and discount.
- **Tax application flow:** `Checkout()` (applies discount) → `ApplyTax()` (applies tax) → `receipt.Total` used everywhere (orders summary, API, etc.).
- **No breaking changes:** The fix to `ApplyTax()` removes the per-line output modification but keeps the same API (`func ApplyTax(r Receipt) Receipt`).
