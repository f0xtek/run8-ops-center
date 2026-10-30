package orders

import (
	"fmt"
	"sort"
)

// Generate computes every available order from the current car status.
//
// It produces at most one order per local job (ID "avail:<job.ID>") and per
// road train (ID "avail:<symbol>"). Generation order is local industry
// lines, local yard lines, road trains with an origin, then through trains.
// Cars that belong to a reserved (In Progress or Completion Detected) order in
// in.Active are excluded everywhere. Available orders may overlap; shared
// cars are listed in Order.Shared with a "shared-cars" flag. The result is
// sorted by Kind and then JobID, so it is deterministic.
func Generate(in GenInput) GenResult {
	if in.Profile == nil || in.Status == nil {
		return GenResult{}
	}
	g := newGenCtx(in)

	var orders []Order
	for i := range in.Profile.LocalJobs {
		if o, ok := g.localOrder(&in.Profile.LocalJobs[i]); ok {
			orders = append(orders, o)
		}
	}
	road, building := g.roadOrders()
	orders = append(orders, road...)

	genMarkShared(orders)
	sort.SliceStable(orders, func(i, j int) bool {
		if orders[i].Kind != orders[j].Kind {
			return orders[i].Kind < orders[j].Kind
		}
		return orders[i].JobID < orders[j].JobID
	})
	return GenResult{Orders: orders, Building: building}
}

// genMarkShared lists, on every order, the car keys that also appear on
// another available order, and adds the "shared-cars" flag.
func genMarkShared(orders []Order) {
	count := map[string]int{}
	for i := range orders {
		for _, k := range orders[i].CarKeys() {
			count[k]++
		}
	}
	for i := range orders {
		var shared []string
		for _, k := range orders[i].CarKeys() {
			if count[k] > 1 {
				shared = append(shared, k)
			}
		}
		if len(shared) == 0 {
			continue
		}
		sort.Strings(shared)
		orders[i].Shared = shared
		orders[i].Flags = append(orders[i].Flags, Flag{
			Code:    "shared-cars",
			Message: fmt.Sprintf("%d car(s) are also listed on another available order; starting one removes them from the other", len(shared)),
		})
	}
}
