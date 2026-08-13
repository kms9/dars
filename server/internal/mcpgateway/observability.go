package mcpgateway

import "time"

// Observer receives bounded, low-cardinality Gateway measurements. Identity
// belongs in redacted activity audit, never in Prometheus labels.
type Observer interface {
	RecordInvocation(sourceKind, outcome string, elapsed time.Duration, inputBytes, outputBytes int)
	RecordSourceValidation(sourceKind, outcome string)
	RecordBundlePublication(outcome string, itemCount int)
}

type nopObserver struct{}

func (nopObserver) RecordInvocation(string, string, time.Duration, int, int) {}
func (nopObserver) RecordSourceValidation(string, string)                    {}
func (nopObserver) RecordBundlePublication(string, int)                      {}

func observerOrNop(observer Observer) Observer {
	if observer == nil {
		return nopObserver{}
	}
	return observer
}
