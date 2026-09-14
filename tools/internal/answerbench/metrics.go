package answerbench

import "os"

// ExportMetrics strips source text, answers, arguments, session IDs and diagnostics.
// Counts and grades remain usable for comparison; private traces own regrading.
func ExportMetrics(input, output string) error {
	raw, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	var report Report
	if err := decodeJSON(raw, &report); err != nil {
		return err
	}
	if err := validateReport(&report); err != nil {
		return err
	}
	tokens, err := reportResultTokens(&report)
	if err != nil {
		return err
	}
	report.MetricsOnly, report.SourceReport, report.ResultTokens = true, digest(raw), &tokens
	for i := range report.Attempts {
		attempt := &report.Attempts[i]
		attempt.Trace.Session, attempt.Trace.Final = "", ""
		if len(attempt.Trace.Errors) > 0 {
			attempt.Trace.Errors = []string{"reader errors retained in private trace"}
		}
		if attempt.Error != "" {
			attempt.Error = "run failed; details retained in private trace"
		}
		if len(attempt.Score.Reasons) > 0 {
			attempt.Score.Reasons = []string{"scoring details retained in private trace"}
		}
		for j := range attempt.Trace.Calls {
			call := &attempt.Trace.Calls[j]
			call.Input, call.Output = nil, ""
			if call.Error != "" {
				call.Error = "tool failed; details retained in private trace"
			}
		}
	}
	if err := validateReport(&report); err != nil {
		return err
	}
	return writeNewJSON(output, &report)
}
