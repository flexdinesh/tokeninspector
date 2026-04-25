package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderTableDaily(t *testing.T) {
	output := renderTable([]renderRow{{
		day:              "2026-04-24",
		provider:         "openai",
		model:            "gpt",
		tpsAvg:           "18.18",
		tpsMean:          "55.00",
		tpsMedian:        "55.00",
		inputTokens:      "300",
		outputTokens:     "30",
		reasoningTokens:  "11",
		cacheReadTokens:  "50",
		cacheWriteTokens: "3",
		totalTokens:      "394",
		requests:         "2",
		retries:          "1",
	}}, groupByNone)

	golden := filepath.Join("testdata", "render_daily.txt")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, []byte(output), 0644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte(output), expected) {
		t.Fatalf("output mismatch\ngot:\n%s\nwant:\n%s", output, expected)
	}
}

func TestRenderTableHourly(t *testing.T) {
	output := renderTable([]renderRow{{
		day:              "2026-04-24",
		hour:             "12:00",
		provider:         "openai",
		model:            "gpt",
		tpsAvg:           "18.18",
		tpsMean:          "55.00",
		tpsMedian:        "55.00",
		inputTokens:      "300",
		outputTokens:     "30",
		reasoningTokens:  "11",
		cacheReadTokens:  "50",
		cacheWriteTokens: "3",
		totalTokens:      "394",
		requests:         "2",
		retries:          "1",
	}}, groupByHour)

	golden := filepath.Join("testdata", "render_hourly.txt")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, []byte(output), 0644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte(output), expected) {
		t.Fatalf("output mismatch\ngot:\n%s\nwant:\n%s", output, expected)
	}
}

func TestRenderTableSession(t *testing.T) {
	output := renderTable([]renderRow{{
		day:              "2026-04-24",
		sessionID:        "session_1234567890",
		thinkingLevels:   "low,high",
		provider:         "openai",
		model:            "openai/gpt-5.5",
		tpsAvg:           "18.18",
		tpsMean:          "55.00",
		tpsMedian:        "55.00",
		inputTokens:      "300",
		outputTokens:     "30",
		reasoningTokens:  "11",
		cacheReadTokens:  "50",
		cacheWriteTokens: "3",
		totalTokens:      "394",
		requests:         "2",
		retries:          "1",
	}}, groupBySession)

	golden := filepath.Join("testdata", "render_session.txt")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, []byte(output), 0644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte(output), expected) {
		t.Fatalf("output mismatch\ngot:\n%s\nwant:\n%s", output, expected)
	}
}
