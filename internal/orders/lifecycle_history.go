package orders

import "sort"

// IsFinished reports whether the order is a history entry: Completed or
// Cancelled. A history entry is the Order itself, so it carries its kind, job,
// cars (Consist and Lines), final State, and the real (StartedAt, FinishedAt)
// and in-game (GameStarted, GameFinished) timestamps.
func IsFinished(o *Order) bool {
	return o.State == StateCompleted || o.State == StateCancelled
}

// History returns the finished orders (Completed or Cancelled) among orders,
// most recently finished first. Orders finished at the same instant keep their
// input order. The input is not modified.
func History(orders []Order) []Order {
	var out []Order
	for i := range orders {
		if IsFinished(&orders[i]) {
			out = append(out, orders[i])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].FinishedAt.After(out[j].FinishedAt) })
	return out
}
