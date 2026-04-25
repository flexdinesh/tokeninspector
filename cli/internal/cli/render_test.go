package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderTableDailyTokens(t *testing.T) {
	output := renderTable([]renderRow{{
		day:              "2026-04-24",
		provider:         "openai",
		model:            "gpt",
		inputTokens:      "300",
		outputTokens:     "30",
		reasoningTokens:  "11",
		cacheReadTokens:  "50",
		cacheWriteTokens: "3",
		totalTokens:      "394",
	}}, groupByNone, tabTokens)

	golden := filepath.Join("testdata", "render_daily_tokens.txt")
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

func TestRenderTableHourlyTokens(t *testing.T) {
	output := renderTable([]renderRow{{
		day:              "2026-04-24",
		hour:             "12:00",
		provider:         "openai",
		model:            "gpt",
		inputTokens:      "300",
		outputTokens:     "30",
		reasoningTokens:  "11",
		cacheReadTokens:  "50",
		cacheWriteTokens: "3",
		totalTokens:      "394",
	}, {
		day:              "2026-04-24",
		hour:             "12:00",
		provider:         "anthropic",
		model:            "claude",
		inputTokens:      "200",
		outputTokens:     "20",
		reasoningTokens:  "5",
		cacheReadTokens:  "10",
		cacheWriteTokens: "1",
		totalTokens:      "236",
	}, {
		day:              "2026-04-24",
		hour:             "13:00",
		provider:         "openai",
		model:            "gpt",
		inputTokens:      "100",
		outputTokens:     "10",
		reasoningTokens:  "0",
		cacheReadTokens:  "0",
		cacheWriteTokens: "0",
		totalTokens:      "110",
	}}, groupByHour, tabTokens)

	golden := filepath.Join("testdata", "render_hourly_tokens.txt")
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

func TestRenderTableSessionTokens(t *testing.T) {
	output := renderTable([]renderRow{{
		day:              "2026-04-24",
		sessionID:        "session_1234567890",
		thinkingLevels:   "low,high",
		provider:         "openai",
		model:            "openai/gpt-5.5",
		inputTokens:      "300",
		outputTokens:     "30",
		reasoningTokens:  "11",
		cacheReadTokens:  "50",
		cacheWriteTokens: "3",
		totalTokens:      "394",
	}}, groupBySession, tabTokens)

	golden := filepath.Join("testdata", "render_session_tokens.txt")
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

func TestRenderTableDailyTPS(t *testing.T) {
	output := renderTable([]renderRow{{
		day:       "2026-04-24",
		provider:  "openai",
		model:     "gpt",
		tpsAvg:    "18.18",
		tpsMean:   "55.00",
		tpsMedian: "55.00",
	}}, groupByNone, tabTPS)

	golden := filepath.Join("testdata", "render_daily_tps.txt")
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

func TestRenderTableSessionRequests(t *testing.T) {
	output := renderTable([]renderRow{{
		day:            "2026-04-24",
		sessionID:      "session_1234567890",
		thinkingLevels: "low,high",
		provider:       "openai",
		model:          "openai/gpt-5.5",
		requests:       "2",
		retries:        "1",
	}}, groupBySession, tabRequests)

	golden := filepath.Join("testdata", "render_session_requests.txt")
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
