package update

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"xci/src/tools/flatpak"
	"xci/src/tools/mise"
)

type toolUpdatePlan struct {
	ToolName     string
	ListOutput   string
	Packages     []updatePackage
	RunUpdate    func([]updatePackage) []packageUpdateResult
	CollectError error
}

type updatePackage struct {
	Name      string
	Requested string
	Current   string
	Latest    string
}

type packageUpdateResult struct {
	Package updatePackage
	Success bool
	Output  string
	Reason  string
}

type toolBackend struct {
	Name          string
	ListOutdated  func() ([]updatePackage, string, error)
	UpdatePackage func([]updatePackage) []packageUpdateResult
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
	backends := []toolBackend{
		miseBackend(),
		flatpakBackend(),
	}

	plans := make([]toolUpdatePlan, 0, len(backends))
	for _, backend := range backends {
		packages, output, err := backend.ListOutdated()
		plans = append(plans, toolUpdatePlan{
			ToolName:     backend.Name,
			ListOutput:   output,
			Packages:     packages,
			RunUpdate:    backend.UpdatePackage,
			CollectError: err,
		})
	}

	return plans
}

func miseBackend() toolBackend {
	return toolBackend{
		Name: "mise",
		ListOutdated: func() ([]updatePackage, string, error) {
			packages, output, err := mise.ListOutdated()
			return mapMiseOutdatedPackages(packages), output, err
		},
		UpdatePackage: func(packages []updatePackage) []packageUpdateResult {
			results := mise.UpdatePackages(mapToMiseOutdatedPackages(packages))
			return mapMiseUpdateResults(results)
		},
	}
}

func flatpakBackend() toolBackend {
	options := flatpak.Options{Scope: flatpak.ScopeUser}

	return toolBackend{
		Name: "flatpak",
		ListOutdated: func() ([]updatePackage, string, error) {
			packages, output, err := flatpak.ListOutdated(options)
			return mapFlatpakOutdatedPackages(packages), output, err
		},
		UpdatePackage: func(packages []updatePackage) []packageUpdateResult {
			results := flatpak.UpdatePackages(options, mapToFlatpakOutdatedPackages(packages))
			return mapFlatpakUpdateResults(results)
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
			fmt.Printf("%d. %s\n", idx+1, describePackagePlanLine(pkg))
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

		results := plan.RunUpdate(plan.Packages)
		outputParts := make([]string, 0, len(results)+1)
		if strings.TrimSpace(summary.Output) != "" {
			outputParts = append(outputParts, fmt.Sprintf("outdated check:\n%s", summary.Output))
		}

		for _, result := range results {
			resultOutput := result.Output
			if strings.TrimSpace(resultOutput) != "" {
				outputParts = append(outputParts, fmt.Sprintf("%s output:\n%s", valueOrUnknown(result.Package.Name), resultOutput))
			}

			if result.Success {
				summary.UpdatedPackages++
				continue
			}

			summary.FailedPackages++
			summary.FailureReasons = append(summary.FailureReasons, fmt.Sprintf("%s: %s", valueOrUnknown(result.Package.Name), result.Reason))
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

func mapMiseOutdatedPackages(packages []mise.OutdatedPackage) []updatePackage {
	out := make([]updatePackage, 0, len(packages))
	for _, pkg := range packages {
		out = append(out, updatePackage{
			Name:      pkg.Name,
			Requested: pkg.Requested,
			Current:   pkg.Current,
			Latest:    pkg.Latest,
		})
	}

	return out
}

func mapToMiseOutdatedPackages(packages []updatePackage) []mise.OutdatedPackage {
	out := make([]mise.OutdatedPackage, 0, len(packages))
	for _, pkg := range packages {
		out = append(out, mise.OutdatedPackage{
			Name:      pkg.Name,
			Requested: pkg.Requested,
			Current:   pkg.Current,
			Latest:    pkg.Latest,
		})
	}

	return out
}

func mapMiseUpdateResults(results []mise.PackageUpdateResult) []packageUpdateResult {
	out := make([]packageUpdateResult, 0, len(results))
	for _, result := range results {
		out = append(out, packageUpdateResult{
			Package: updatePackage{
				Name:      result.Package.Name,
				Requested: result.Package.Requested,
				Current:   result.Package.Current,
				Latest:    result.Package.Latest,
			},
			Success: result.Success,
			Output:  result.Output,
			Reason:  result.Reason,
		})
	}

	return out
}

func mapFlatpakOutdatedPackages(packages []flatpak.OutdatedPackage) []updatePackage {
	out := make([]updatePackage, 0, len(packages))
	for _, pkg := range packages {
		out = append(out, updatePackage{Name: pkg.Ref})
	}

	return out
}

func mapToFlatpakOutdatedPackages(packages []updatePackage) []flatpak.OutdatedPackage {
	out := make([]flatpak.OutdatedPackage, 0, len(packages))
	for _, pkg := range packages {
		out = append(out, flatpak.OutdatedPackage{Ref: pkg.Name})
	}

	return out
}

func mapFlatpakUpdateResults(results []flatpak.PackageUpdateResult) []packageUpdateResult {
	out := make([]packageUpdateResult, 0, len(results))
	for _, result := range results {
		out = append(out, packageUpdateResult{
			Package: updatePackage{Name: result.Package.Ref},
			Success: result.Success,
			Output:  result.Output,
			Reason:  result.Reason,
		})
	}

	return out
}

func describePackagePlanLine(pkg updatePackage) string {
	name := strings.TrimSpace(pkg.Name)
	if name == "" {
		name = "unknown"
	}

	requested := strings.TrimSpace(pkg.Requested)
	current := strings.TrimSpace(pkg.Current)
	latest := strings.TrimSpace(pkg.Latest)

	if current == "" && latest == "" {
		if requested == "" {
			return name
		}

		return fmt.Sprintf("%s (requested: %s)", name, requested)
	}

	line := fmt.Sprintf("%s %s -> %s", name, valueOrUnknown(current), valueOrUnknown(latest))
	if requested != "" {
		line = fmt.Sprintf("%s (requested: %s)", line, requested)
	}

	return line
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
