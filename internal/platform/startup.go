package platform

// Startup is whether Seaglass starts when you sign in to Windows.
type Startup struct {
	On bool `json:"on"`
	// Off in Task Manager's Startup apps: Windows skips it even though
	// it's registered.
	DisabledByUser bool `json:"disabledByUser"`
}
