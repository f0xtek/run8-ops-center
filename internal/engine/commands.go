package engine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"run8-ops-center/internal/orders"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/trackmap"
)

// finish persists state and publishes. Commands call it last, so a command
// returns only after its View is visible.
func (e *Engine) finish() error {
	err := e.persist()
	e.publish()
	return err
}

// StartOrder starts an available order, or one line of a local order.
func (e *Engine) StartOrder(id, lineKey string) (orders.Order, error) {
	var out orders.Order
	err := e.do(func() error {
		var avail *orders.Order
		for i := range e.avail {
			if e.avail[i].ID == id {
				avail = &e.avail[i]
				break
			}
		}
		if avail == nil {
			return fmt.Errorf("order %q is not available; it may have been started or regenerated", id)
		}
		o, err := orders.Start(*avail, lineKey, fmt.Sprintf("ord-%d", e.state.Seq+1), e.clock())
		if err != nil {
			return err
		}
		e.state.Seq++
		list := append(slices.Clone(e.orders()), o)
		e.setOrders(list)
		e.logf("order", "Started %s", orderLabel(o))
		out = cloneOrder(o)
		e.recompute(false)
		return e.finish()
	})
	return out, err
}

// ApplyOrder confirms, continues, completes or cancels a started order of the
// active profile.
func (e *Engine) ApplyOrder(id string, a orders.Action) (orders.Order, error) {
	var out orders.Order
	err := e.do(func() error {
		list := e.orders()
		i := slices.IndexFunc(list, func(o orders.Order) bool { return o.ID == id })
		if i < 0 {
			return fmt.Errorf("order %q not found", id)
		}
		o, err := orders.Apply(list[i], a, e.clock())
		if err != nil {
			return err
		}
		list = slices.Clone(list)
		list[i] = o
		e.setOrders(list)
		switch a {
		case orders.ActConfirm:
			e.logf("order", "Completed %s (confirmed)", orderLabel(o))
		case orders.ActComplete:
			e.logf("order", "Completed %s (marked done by hand)", orderLabel(o))
		case orders.ActCancel:
			e.logf("order", "Cancelled %s", orderLabel(o))
		case orders.ActContinue:
			e.logf("order", "%s back in progress", orderLabel(o))
		}
		out = cloneOrder(o)
		e.recompute(false)
		return e.finish()
	})
	return out, err
}

// LabelCandidate labels every unmapped section of the learn-mode candidate
// for trainID.
func (e *Engine) LabelCandidate(trainID string, l trackmap.Label) error {
	return e.do(func() error {
		l, err := normaliseLabel(l)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(e.candidates, func(c trackmap.Candidate) bool { return c.TrainID == trainID })
		if i < 0 {
			return fmt.Errorf("train %q has no unmapped sections in the current save", trainID)
		}
		tm := e.tm.Clone()
		tm.Apply(e.candidates[i], l)
		return e.commitMap(tm)
	})
}

// LabelSection labels one section, replacing any label it has.
func (e *Engine) LabelSection(prefix, section int, l trackmap.Label) error {
	return e.do(func() error {
		l, err := normaliseLabel(l)
		if err != nil {
			return err
		}
		tm := e.tm.Clone()
		tm.Set(prefix, section, l)
		return e.commitMap(tm)
	})
}

// RemoveLabel makes a section unmapped again.
func (e *Engine) RemoveLabel(prefix, section int) error {
	return e.do(func() error {
		if _, ok := e.tm.Lookup(prefix, section); !ok {
			return fmt.Errorf("section %s has no label", trackmap.Key(prefix, section))
		}
		tm := e.tm.Clone()
		tm.Remove(prefix, section)
		return e.commitMap(tm)
	})
}

// RenameLocation relabels every section of old as new and returns how many
// sections changed.
func (e *Engine) RenameLocation(old, new trackmap.Location) (int, error) {
	var n int
	err := e.do(func() error {
		l, err := normaliseLabel(trackmap.Label(new))
		if err != nil {
			return err
		}
		tm := e.tm.Clone()
		n = tm.Rename(old, l.Location())
		if n == 0 {
			return nil
		}
		return e.commitMap(tm)
	})
	return n, err
}

// commitMap saves tm as the active profile's track map and, only when that
// worked, adopts it and recomputes from the last snapshot. Labels change car
// locations, so order progress is reconciled too.
func (e *Engine) commitMap(tm *trackmap.Map) error {
	if err := tm.Save(e.guard, e.mapPath()); err != nil {
		return err
	}
	e.tm = tm
	e.recompute(true)
	return e.finish()
}

func (e *Engine) mapPath() string { return filepath.Join(e.base.Dir, trackMapFile) }

// normaliseLabel trims the label and rejects incomplete ones.
func normaliseLabel(l trackmap.Label) (trackmap.Label, error) {
	out := trackmap.Label{Kind: l.Kind}
	switch l.Kind {
	case trackmap.KindYard:
		out.Yard = strings.TrimSpace(l.Yard)
		out.Track = strings.TrimSpace(l.Track)
		if out.Yard == "" || out.Track == "" {
			return out, errors.New("a yard label needs a yard and a track name")
		}
	case trackmap.KindIndustry:
		for _, t := range l.Industries {
			if t = strings.TrimSpace(t); t != "" && !slices.Contains(out.Industries, t) {
				out.Industries = append(out.Industries, t)
			}
		}
		if len(out.Industries) == 0 {
			return out, errors.New("an industry label needs at least one industry tag")
		}
	case trackmap.KindIgnored:
	default:
		return out, fmt.Errorf("unknown label kind %q", l.Kind)
	}
	return out, nil
}

// SetThreshold sets or clears the per-train overrides. A nil pointer clears
// that override.
func (e *Engine) SetThreshold(symbol string, minLengthPct, maxCarAgeDays *float64) error {
	return e.do(func() error {
		if e.base.RoadTrainBySymbol(symbol) == nil {
			return fmt.Errorf("profile %s has no road train %q", e.base.ID, symbol)
		}
		if minLengthPct != nil && (*minLengthPct < 0 || *minLengthPct > 100) {
			return fmt.Errorf("minimum length must be between 0 and 100 percent, got %v", *minLengthPct)
		}
		if maxCarAgeDays != nil && *maxCarAgeDays < 0 {
			return fmt.Errorf("maximum car age cannot be negative, got %v", *maxCarAgeDays)
		}
		s := cloneSettings(e.settings)
		th := cloneThreshold(store.Threshold{MinLengthPct: minLengthPct, MaxCarAgeDays: maxCarAgeDays})
		if th.MinLengthPct == nil && th.MaxCarAgeDays == nil {
			delete(s.Thresholds, symbol)
			if len(s.Thresholds) == 0 {
				s.Thresholds = nil
			}
		} else {
			if s.Thresholds == nil {
				s.Thresholds = map[string]store.Threshold{}
			}
			s.Thresholds[symbol] = th
		}
		if err := e.saveSettings(s); err != nil {
			return err
		}
		e.eff = e.effectiveProfile()
		e.recompute(false)
		return e.finish()
	})
}

// SetInstallDir sets the optional Run8 install folder, validated by the guard.
// An empty dir clears it.
func (e *Engine) SetInstallDir(dir string) error {
	return e.do(func() error {
		old := e.guard.InstallDir()
		if err := e.guard.SetInstallDir(dir); err != nil {
			return err
		}
		s := e.settings
		s.RunInstallDir = dir
		if err := e.saveSettings(s); err != nil {
			_ = e.guard.SetInstallDir(old) // old was accepted before, so this cannot conflict
			return err
		}
		e.loadRoute()
		e.recompute(false)
		return e.finish()
	})
}

// SetServer stores the HTTP port and bind option. They take effect on restart.
func (e *Engine) SetServer(port int, allInterfaces bool) error {
	return e.do(func() error {
		if port < 1 || port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535, got %d", port)
		}
		s := e.settings
		s.Port, s.AllInterfaces = port, allInterfaces
		if err := e.saveSettings(s); err != nil {
			return err
		}
		return e.finish()
	})
}

// saveSettings writes s and, only on success, adopts it.
func (e *Engine) saveSettings(s store.Settings) error {
	if err := store.SaveSettings(e.guard, e.dataDir, s); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	e.settings = s
	return nil
}

// SwitchProfile makes another installed profile active. The profile is
// reloaded from disk with its own track map, orders and dwell records. Orders
// of the profile being left stay in state.json and come back when the user
// switches back.
func (e *Engine) SwitchProfile(id string) error {
	return e.do(func() error {
		found, problems := scanProfiles(e.profilesDir)
		target := pickByID(found, id)
		if target == nil {
			return fmt.Errorf("profile %q is not installed in %s", id, e.profilesDir)
		}
		tm, err := e.prepareProfile(target)
		if err != nil {
			return err
		}
		s := e.settings
		s.ProfileID = id
		if err := e.saveSettings(s); err != nil {
			return err
		}
		e.installProfile(target, tm)
		e.installed = infoOf(found)
		e.dropWarnings("profile")
		for _, m := range problems {
			e.addWarning("profile", m)
		}
		e.loadRoute()
		e.recompute(true)
		return e.finish()
	})
}

func pickByID(found []*profile.Profile, id string) *profile.Profile {
	for _, p := range found {
		if p.ID == id {
			return p
		}
	}
	return nil
}
