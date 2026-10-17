package workset

// LightPlanner declares that the whole generation path avoids inventory scans
// and media probes. Declared tasks may use one extra worker; general workers
// still claim them in FIFO order. Tasks without this declaration stay general.
type LightPlanner interface {
	LightPlanning()
}
