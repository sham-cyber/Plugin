package main

import (
	"fmt"
	"time"

	"github.com/maximhq/bifrost/plugins/fraudguard"
)

func main() {
	engine, err := fraudguard.NewEngine(fraudguard.Config{WarmupRequests: 4, AlertThreshold: 50})
	if err != nil {
		panic(err)
	}
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		record(engine, "demo-internal-vk", "openai", "model-a", base.Add(time.Duration(i)*30*time.Second), 250, 80)
	}
	handle, assessment := engine.Start(fraudguard.Request{
		VirtualKeyID: "demo-internal-vk",
		Provider:     "openai",
		Model:        "model-a",
		Timestamp:    base.Add(121 * time.Second),
	})
	engine.Complete(handle, fraudguard.Completion{
		InputTokens:  250,
		OutputTokens: 80,
		Timestamp:    base.Add(121 * time.Second),
	})
	fmt.Printf("observe-only finding: score=%d alert=%t signals=%v\n",
		assessment.Finding.Score, assessment.Finding.Alert, signalNames(assessment.Finding.Signals))
	fmt.Printf("metrics: %+v\n", engine.Metrics())
}

func record(engine *fraudguard.Engine, key, provider, model string, at time.Time, input, output int64) {
	handle, _ := engine.Start(fraudguard.Request{
		VirtualKeyID: key,
		Provider:     provider,
		Model:        model,
		Timestamp:    at,
	})
	engine.Complete(handle, fraudguard.Completion{
		InputTokens:  input,
		OutputTokens: output,
		Timestamp:    at,
	})
}

func signalNames(signals []fraudguard.Signal) []string {
	names := make([]string, 0, len(signals))
	for _, signal := range signals {
		names = append(names, signal.Name)
	}
	return names
}
