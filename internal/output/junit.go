package output

import (
	"encoding/xml"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
)

func init() {
	RegisterFormatter("junit", &JUnitFormatter{})
}

// JUnitFormatter formats health check responses as JUnit XML.
type JUnitFormatter struct{}

// JUnit XML structures

type junitTestSuite struct {
	XMLName   xml.Name         `xml:"testsuite"`
	Name      string           `xml:"name,attr"`
	Tests     int              `xml:"tests,attr"`
	Failures  int              `xml:"failures,attr"`
	Errors    int              `xml:"errors,attr"`
	Skipped   int              `xml:"skipped,attr"`
	Time      float64          `xml:"time,attr"`
	Timestamp string           `xml:"timestamp,attr,omitempty"`
	Suites    []junitTestSuite `xml:"testsuite,omitempty"`
	Cases     []junitTestCase  `xml:"testcase,omitempty"`
}

type junitTestCase struct {
	XMLName   xml.Name       `xml:"testcase"`
	Name      string         `xml:"name,attr"`
	Classname string         `xml:"classname,attr"`
	Time      float64        `xml:"time,attr"`
	Failures  []junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped  `xml:"skipped,omitempty"`
	SystemOut *junitCDATA    `xml:"system-out,omitempty"`
}

type junitCDATA struct {
	Content string `xml:",cdata"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Content string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr,omitempty"`
}

// Format converts the health check response to JUnit XML.
func (f *JUnitFormatter) Format(status *ph.HealthCheckResponse, cfg Config) ([]byte, error) {
	suite := buildTestSuite(status, cfg.Flat)
	suite.Timestamp = time.Now().UTC().Format(time.RFC3339)

	output, err := xml.MarshalIndent(suite, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), output...), nil
}

// buildTestSuite builds the root testsuite from a HealthCheckResponse.
func buildTestSuite(resp *ph.HealthCheckResponse, flat bool) junitTestSuite {
	suite := junitTestSuite{
		Name: resp.Name,
	}

	if len(resp.Components) == 0 {
		// Single check with no components - the root is the testcase
		suite.Cases = []junitTestCase{buildTestCase(resp, "")}
	} else if flat {
		// Flat mode: all components as testcases with path-based names
		suite.Cases = collectFlatTestCases(resp.Components, "")
	} else {
		// Hierarchical mode: nested testsuites for systems
		buildHierarchical(&suite, resp.Components)
	}

	// Calculate totals
	calculateTotals(&suite)

	return suite
}

// buildHierarchical builds nested testsuites and testcases.
func buildHierarchical(parent *junitTestSuite, components []*ph.HealthCheckResponse) {
	for _, comp := range components {
		if len(comp.Components) > 0 {
			// This is a system/container - create nested testsuite
			childSuite := junitTestSuite{
				Name: comp.Name,
			}
			buildHierarchical(&childSuite, comp.Components)
			calculateTotals(&childSuite)
			parent.Suites = append(parent.Suites, childSuite)
		} else {
			// This is a leaf - create testcase
			parent.Cases = append(parent.Cases, buildTestCase(comp, ""))
		}
	}
}

// collectFlatTestCases collects all leaf components as flat testcases with path names.
func collectFlatTestCases(components []*ph.HealthCheckResponse, prefix string) []junitTestCase {
	var cases []junitTestCase
	for _, comp := range components {
		name := comp.Name
		if prefix != "" {
			name = prefix + "/" + comp.Name
		}

		if len(comp.Components) > 0 {
			// Recurse into children
			cases = append(cases, collectFlatTestCases(comp.Components, name)...)
		} else {
			// Leaf node - create testcase
			cases = append(cases, buildTestCase(comp, prefix))
		}
	}
	return cases
}

// buildTestCase creates a JUnit testcase from a health check response.
func buildTestCase(resp *ph.HealthCheckResponse, prefix string) junitTestCase {
	name := resp.Name
	if prefix != "" {
		name = prefix + "/" + resp.Name
	}

	tc := junitTestCase{
		Name:      name,
		Classname: resp.Type,
		Time:      durationSeconds(resp.Duration),
	}

	// Render details for system-out (all checks, for audit purposes)
	detailText := details.RenderAll(resp.Details, "")
	if detailText != "" {
		tc.SystemOut = &junitCDATA{Content: detailText}
	}

	switch resp.Status {
	case ph.Status_UNHEALTHY:
		// Build failure content with messages and details
		content := buildFailureContent(resp.Messages, detailText)
		message := "UNHEALTHY"
		if len(resp.Messages) > 0 {
			message = resp.Messages[0]
		}
		tc.Failures = append(tc.Failures, junitFailure{
			Message: message,
			Content: content,
		})

	case ph.Status_UNKNOWN:
		msg := "UNKNOWN"
		if len(resp.Messages) > 0 {
			msg = resp.Messages[0]
		}
		tc.Skipped = &junitSkipped{Message: msg}

	case ph.Status_LOOP_DETECTED:
		// Include loop details in skipped message if available
		msg := "LOOP_DETECTED"
		if detailText != "" {
			msg = "LOOP_DETECTED\n" + detailText
		}
		tc.Skipped = &junitSkipped{Message: msg}
	}

	return tc
}

// buildFailureContent creates rich diagnostic content for failures.
func buildFailureContent(messages []string, detailText string) string {
	var sb strings.Builder

	if len(messages) > 0 {
		for i, msg := range messages {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(msg)
		}
	}

	if detailText != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\nDetails:\n")
		}
		sb.WriteString(detailText)
	}

	content := sb.String()
	if content == "" {
		return "UNHEALTHY"
	}
	return content
}

// calculateTotals recursively calculates test/failure/skipped counts and total time.
func calculateTotals(suite *junitTestSuite) {
	suite.Tests = 0
	suite.Failures = 0
	suite.Skipped = 0
	suite.Errors = 0
	suite.Time = 0

	// Count from testcases
	for _, tc := range suite.Cases {
		suite.Tests++
		suite.Time += tc.Time
		if len(tc.Failures) > 0 {
			suite.Failures++
		}
		if tc.Skipped != nil {
			suite.Skipped++
		}
	}

	// Aggregate from child suites
	for _, child := range suite.Suites {
		suite.Tests += child.Tests
		suite.Failures += child.Failures
		suite.Skipped += child.Skipped
		suite.Errors += child.Errors
		suite.Time += child.Time
	}
}

// durationSeconds converts a protobuf Duration to seconds as float64.
func durationSeconds(d *durationpb.Duration) float64 {
	if d == nil {
		return 0
	}
	return float64(d.Seconds) + float64(d.Nanos)/1e9
}
