package web

import "run8-ops-center/internal/engine"

// viewAdapter gives the Server the Backend it needs from the engine's View.
type viewAdapter struct{ b engine.Backend }

// NewBackendAdapter returns the [Backend] for a Server, built from the engine. The
// header shows the save name, in-game date, last ingest and the source folders from the
// current View. Its warning count is the length of the dashboard's attention list, so
// the badge and the list always agree.
func NewBackendAdapter(b engine.Backend) Backend { return viewAdapter{b} }

func (a viewAdapter) Chrome() Chrome {
	v := a.b.View()
	if v == nil {
		return Chrome{}
	}
	return Chrome{
		SaveName:     v.Source.SaveName,
		ActiveFile:   v.Source.ActiveFile,
		GameDate:     v.Source.GameDate,
		LastIngest:   v.Source.LastIngest,
		SaveDir:      v.Source.SaveDir,
		AutoSaveDir:  v.Source.AutoSaveDir,
		WarningCount: len(dashAttention(v)),
	}
}

func (a viewAdapter) Subscribe() (<-chan struct{}, func()) { return a.b.Subscribe() }
