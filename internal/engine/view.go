package engine

import (
	"slices"

	"run8-ops-center/internal/orders"
)

// publish builds an immutable View from the engine's state, stores it and
// wakes subscribers. Everything the View points at is either freshly built
// here or replaced (never mutated) by later steps.
func (e *Engine) publish() {
	e.viewSeq++
	list := e.orders()
	var active []orders.Order
	rev := make([]orders.Order, 0, len(list))
	for i := len(list) - 1; i >= 0; i-- { // newest first
		rev = append(rev, list[i])
		if list[i].Reserved() {
			active = append(active, list[i])
		}
	}
	v := &View{
		Seq:         e.viewSeq,
		Source:      e.source,
		HasSnapshot: e.snap != nil,
		Profile:     e.eff,
		Profiles:    slices.Clone(e.installed),
		Status:      e.status,
		Available:   e.avail,
		Active:      active,
		History:     orders.History(rev),
		Building:    e.building,
		Tiles:       tiles(e.status),
		Warnings:    e.warnings,
		TrackMap:    e.tm,
		Candidates:  e.candidates,
		Locations:   e.locations,
		Coverage:    e.coverage,
		Activity:    e.state.Activity,
		Settings:    cloneSettings(e.settings),
	}
	e.view.Store(v)
	e.notify()
}

// cloneOrder deep-copies an order's slices.
func cloneOrder(o orders.Order) orders.Order {
	o.Consist = slices.Clone(o.Consist)
	if o.Lines != nil {
		lines := make([]orders.Line, len(o.Lines))
		for i, l := range o.Lines {
			l.SetOuts = slices.Clone(l.SetOuts)
			l.PickUps = slices.Clone(l.PickUps)
			lines[i] = l
		}
		o.Lines = lines
	}
	o.Flags = slices.Clone(o.Flags)
	o.Shared = slices.Clone(o.Shared)
	return o
}
