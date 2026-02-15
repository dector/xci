package update

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"xci/src/tools/mise"
)

type toolUpdatePlan struct {
	ToolName     string
	ListOutput   string
	Packages     []mise.OutdatedPackage
	CollectError error
}

type toolUpdateSummary struct {
	ToolName        string
	Status          string
	Output          string
	UpdatedPackages int
	FailedPackages  int
	FailureReasons  []string
}

func Run() error {
	fmt.Println("Checking for available updates...")

	plans := collectUpdatePlans()
	printUpdatePlanSections(plans)

	if hasCollectionFailures(plans) {
		summaries := collectionFailureSummaries(plans)
		printUpdateSummarySections(summaries)
		return fmt.Errorf("failed to collect update list")
	}

	totalPackages := 0
	for _, plan := range plans {
		totalPackages += len(plan.Packages)
	}

	if totalPackages == 0 {
		summaries := noUpdateSummaries(plans)
		printUpdateSummarySections(summaries)
		return nil
	}

	confirmed, err := askUserConfirmation("\nApply these updates now?")
	if err != nil {
		return err
	}

	if !confirmed {
		summaries := cancelledSummaries(plans)
		printUpdateSummarySections(summaries)
		return nil
	}

	fmt.Println("\nApplying updates...")
	summaries := executeUpdates(plans)
	printUpdateSummarySections(summaries)

	for _, summary := range summaries {
		if summary.FailedPackages > 0 {
			return fmt.Errorf("one or more updates failed")
		}
	}

	return nil
}

func collectUpdatePlans() []toolUpdatePlan {
	packages, output, err := mise.ListOutdated()

	return []toolUpdatePlan{
		{
			ToolName:     "mise",
			ListOutput:   output,
			Packages:     packages,
			CollectError: err,
		},
	}
}

func printUpdatePlanSections(plans []toolUpdatePlan) {
	fmt.Println("\nUpdate plan:")

	for _, plan := range plans {
		fmt.Printf("\n[%s]\n", plan.ToolName)
		if plan.CollectError != nil {
			fmt.Printf("Could not collect outdated packages: %v\n", plan.CollectError)
			continue
		}

		if len(plan.Packages) == 0 {
			fmt.Println("No packages need an update.")
			continue
		}

		fmt.Printf("%d package(s) can be updated:\n", len(plan.Packages))

		for idx, pkg := range plan.Packages {
			fmt.Printf("%d. %-16s %s -> %s", idx+1, pkg.Name, valueOrUnknown(pkg.Current), valueOrUnknown(pkg.Latest))
			if strings.TrimSpace(pkg.Requested) != "" {
				fmt.Printf(" (requested: %s)", pkg.Requested)
			}
			fmt.Println()
		}
	}
}

func hasCollectionFailures(plans []toolUpdatePlan) bool {
	for _, plan := range plans {
		if plan.CollectError != nil {
			return true
		}
	}

	return false
}

func collectionFailureSummaries(plans []toolUpdatePlan) []toolUpdateSummary {
	summaries := make([]toolUpdateSummary, 0, len(plans))
	for _, plan := range plans {
		summary := toolUpdateSummary{
			ToolName:        plan.ToolName,
			Status:          "error",
			Output:          plan.ListOutput,
			UpdatedPackages: 0,
			FailedPackages:  0,
		}

		if plan.CollectError != nil {
			summary.FailureReasons = append(summary.FailureReasons, plan.CollectError.Error())
		}

		summaries = append(summaries, summary)
	}

	return summaries
}

func noUpdateSummaries(plans []toolUpdatePlan) []toolUpdateSummary {
	summaries := make([]toolUpdateSummary, 0, len(plans))
	for _, plan := range plans {
		summaries = append(summaries, toolUpdateSummary{
			ToolName:        plan.ToolName,
			Status:          "up-to-date",
			Output:          plan.ListOutput,
			UpdatedPackages: 0,
			FailedPackages:  0,
		})
	}

	return summaries
}

func cancelledSummaries(plans []toolUpdatePlan) []toolUpdateSummary {
	summaries := make([]toolUpdateSummary, 0, len(plans))
	for _, plan := range plans {
		summaries = append(summaries, toolUpdateSummary{
			ToolName:        plan.ToolName,
			Status:          "cancelled",
			Output:          appendOutput("update cancelled by user", plan.ListOutput),
			UpdatedPackages: 0,
			FailedPackages:  0,
		})
	}

	return summaries
}

func executeUpdates(plans []toolUpdatePlan) []toolUpdateSummary {
	summaries := make([]toolUpdateSummary, 0, len(plans))

	for _, plan := range plans {
		summary := toolUpdateSummary{
			ToolName: plan.ToolName,
			Output:   plan.ListOutput,
		}

		if len(plan.Packages) == 0 {
			summary.Status = "up-to-date"
			summaries = append(summaries, summary)
			continue
		}

		results := mise.UpdatePackages(plan.Packages)
		outputParts := make([]string, 0, len(results)+1)
		if strings.TrimSpace(summary.Output) != "" {
			outputParts = append(outputParts, fmt.Sprintf("outdated check:\n%s", summary.Output))
		}

		for _, result := range results {
			resultOutput := result.Output
			if strings.TrimSpace(resultOutput) != "" {
				outputParts = append(outputParts, fmt.Sprintf("%s output:\n%s", result.Package.Name, resultOutput))
			}

			if result.Success {
				summary.UpdatedPackages++
				continue
			}

			summary.FailedPackages++
			summary.FailureReasons = append(summary.FailureReasons, fmt.Sprintf("%s: %s", result.Package.Name, result.Reason))
		}

		summary.Output = strings.Join(outputParts, "\n\n")
		summary.Status = "updated"
		if summary.FailedPackages > 0 && summary.UpdatedPackages > 0 {
			summary.Status = "partial"
		}
		if summary.FailedPackages > 0 && summary.UpdatedPackages == 0 {
			summary.Status = "failed"
		}

		summaries = append(summaries, summary)
	}

	return summaries
}

func printUpdateSummarySections(summaries []toolUpdateSummary) {
	fmt.Println("\nUpdate results:")

	totalUpdated := 0
	totalFailed := 0

	for _, summary := range summaries {
		totalUpdated += summary.UpdatedPackages
		totalFailed += summary.FailedPackages

		fmt.Printf("\n[%s]\n", summary.ToolName)
		fmt.Printf("Result: %s\n", describeStatus(summary.Status))
		fmt.Printf("Updated: %d\n", summary.UpdatedPackages)
		fmt.Printf("Failed: %d\n", summary.FailedPackages)

		if len(summary.FailureReasons) > 0 {
			fmt.Println("Failure reasons:")
			for _, reason := range summary.FailureReasons {
				fmt.Printf("- %s\n", reason)
			}
		}

		fmt.Println("Output:")
		if strings.TrimSpace(summary.Output) == "" {
			fmt.Println("(no output captured)")
			continue
		}

		printIndented(summary.Output, "  ")
	}

	fmt.Println("\nOverall:")
	fmt.Printf("Updated: %d\n", totalUpdated)
	fmt.Printf("Failed: %d\n", totalFailed)
}

func askUserConfirmation(prompt string) (bool, error) {
	fmt.Printf("%s [y/N]: ", prompt)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes", nil
}

func appendOutput(prefix, output string) string {
	prefix = strings.TrimSpace(prefix)
	output = strings.TrimSpace(output)
	if prefix == "" {
		return output
	}
	if output == "" {
		return prefix
	}

	return fmt.Sprintf("%s\n%s", prefix, output)
}

func valueOrUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}

	return value
}

func describeStatus(status string) string {
	switch status {
	case "updated":
		return "updated successfully"
	case "partial":
		return "partially updated"
	case "failed":
		return "update failed"
	case "cancelled":
		return "cancelled by user"
	case "up-to-date":
		return "already up to date"
	case "error":
		return "could not prepare update"
	default:
		return status
	}
}

func printIndented(text, indent string) {
	for _, line := range strings.Split(text, "\n") {
		fmt.Printf("%s%s\n", indent, line)
	}
}
