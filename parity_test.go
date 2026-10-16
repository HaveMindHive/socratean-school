package main

// Parity harness: exposes the private analysis funcs on a fixed set of sample
// texts and writes results as JSON for comparison against the JS port.
// Run: go test -run TestParityDump -v

import (
	"encoding/json"
	"os"
	"testing"
)

var paritySamples = []string{
	"I honestly think the whole thing is kind of weird. I mean, why would anyone do that? I don't get it, but sort of respect the hustle. Honestly it's wild.",
	"Furthermore, the comprehensive process leverages a robust and seamless framework. Moreover, it is important to note that we utilize best practices to optimize outcomes. In conclusion, this innovative paradigm plays a crucial role.",
	"The cat sat.",
	"When I was young, my family moved a lot, and I felt scared every time. We packed our things, said goodbye to our friends, and drove for hours. I remember crying in the back seat, worried I would never fit in anywhere.",
	"So the light hits the chlorophyll and gets absorbed, kind of like a sponge soaking up water. So the plant is basically using sunlight to build sugar molecules, like a tiny factory that never stops working through the day.",
}

func TestParityDump(t *testing.T) {
	base := analyzeWritingStyle(paritySamples[0])
	type row struct {
		FP        WritingFingerprint `json:"fingerprint"`
		AIScore   float64            `json:"ai_score"`
		CompScore float64            `json:"comprehensive"`
		Style0    float64            `json:"style_vs_sample0"`
	}
	out := make([]row, 0, len(paritySamples))
	for _, s := range paritySamples {
		out = append(out, row{
			FP:        analyzeWritingStyle(s),
			AIScore:   detectAIPatterns(s),
			CompScore: GetComprehensiveAIScore(s),
			Style0:    compareToFingerprint(s, base),
		})
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile("/Users/paymore/HaveMindHive/socratean-witness_v1.0_08-17-2026/test/go_out.json", b, 0644); err != nil {
		t.Fatal(err)
	}
}
