// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

func (e *Engine) enqueueFindingCallback(finding Finding) {
	if e.findingQueue == nil || len(finding.Signals) == 0 {
		return
	}

	e.findingMu.Lock()
	if e.findingClosed {
		e.findingMu.Unlock()
		e.recordFindingCallback(false, true)
		return
	}
	e.mu.Lock()
	select {
	case e.findingQueue <- cloneFinding(finding):
		e.metrics.FindingCallbacksQueued++
		e.mu.Unlock()
		e.findingMu.Unlock()
	default:
		e.mu.Unlock()
		e.findingMu.Unlock()
		e.recordFindingCallback(false, true)
	}
}

func (e *Engine) runFindingWorker() {
	defer e.findingWG.Done()
	for finding := range e.findingQueue {
		if callbackPanicked(e.cfg.OnFinding, finding) {
			e.recordFindingCallback(false, false)
			continue
		}
		e.recordFindingCallback(true, false)
	}
}

func callbackPanicked(callback func(Finding), finding Finding) (panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	callback(cloneFinding(finding))
	return false
}

func (e *Engine) recordFindingCallback(completed, dropped bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case completed:
		e.metrics.FindingCallbacksCompleted++
	case dropped:
		e.metrics.FindingCallbacksDropped++
	default:
		e.metrics.FindingCallbacksFailed++
	}
}
