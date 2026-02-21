package update

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"xci/internal/utils"
	"xci/src/tools/dnf5"
	"xci/src/tools/flatpak"
	"xci/src/tools/mise"
)

var (
	lookPathFunc                     = exec.LookPath
	detectDistroFamilyFunc           = utils.DetectDistroFamily
	terminalColumnsFunc              = detectTerminalColumns
	progressOutputWriter   io.Writer = os.Stdout
	colorOutputEnabled               = supportsColorOutput()
	ansiEscapePattern                = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	loaderFrames                     = []string{"Ooo", "oOo", "ooO"}
)

const (
	ansiReset    = "\033[0m"
	ansiBlue     = "\033[34m"
	ansiGreen    = "\033[32m"
	ansiYellow   = "\033[33m"
	ansiRed      = "\033[31m"
	ansiBoldCyan = "\033[1;36m"

	defaultTerminalColumns = 80
	frameHorizontalPadding = 4
	loaderTickInterval     = 150 * time.Millisecond
	loaderThirdFrameDelay  = 500 * time.Millisecond
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
	UpdatedNames    []string
	FailedPackages  int
	FailureReasons  []string
}

type updateCollectResult struct {
	index int
	plan  toolUpdatePlan
}

type progressReporter interface {
	Start()
	AdvanceFrame()
	MarkDone(index, packages int)
}

func Run() error {
	fmt.Println("Checking for available updates...")

	plans := collectUpdatePlansWithProgress()
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

func collectUpdatePlansWithProgress() []toolUpdatePlan {
	backends := registeredBackends()
	reporter := newUpdateProgressRenderer(backends, progressOutputWriter, supportsInPlaceProgressOutput())

	return collectUpdatePlansWithProgressForBackends(backends, reporter)
}

func collectUpdatePlansWithProgressForBackends(backends []toolBackend, reporter progressReporter) []toolUpdatePlan {
	plans := make([]toolUpdatePlan, len(backends))
	if len(backends) == 0 {
		return plans
	}

	if reporter != nil {
		reporter.Start()
	}

	results := make(chan updateCollectResult, len(backends))
	for idx, backend := range backends {
		go func(index int, backend toolBackend) {
			packages, output, err := backend.ListOutdated()
			results <- updateCollectResult{
				index: index,
				plan: toolUpdatePlan{
					ToolName:     backend.Name,
					ListOutput:   output,
					Packages:     packages,
					RunUpdate:    backend.UpdatePackage,
					CollectError: err,
				},
			}
		}(idx, backend)
	}

	ticker := time.NewTicker(loaderTickInterval)
	defer ticker.Stop()

	for completed := 0; completed < len(backends); {
		select {
		case result := <-results:
			plans[result.index] = result.plan
			if reporter != nil {
				reporter.MarkDone(result.index, len(result.plan.Packages))
			}
			completed++
		case <-ticker.C:
			if reporter != nil {
				reporter.AdvanceFrame()
			}
		}
	}

	return plans
}

func registeredBackends() []toolBackend {
	backends := []toolBackend{
		miseBackend(),
		flatpakBackend(),
	}

	if shouldIncludeDNF5Backend() {
		backends = append(backends, dnf5Backend())
	}

	return backends
}

func shouldIncludeDNF5Backend() bool {
	_, err := lookPathFunc("dnf5")
	if err == nil {
		return true
	}

	if !errors.Is(err, exec.ErrNotFound) {
		return true
	}

	return detectDistroFamilyFunc() == utils.DistroFamilyFedoraLike
}

type updateProgressRow struct {
	toolName     string
	done         bool
	packageCount int
	startedAt    time.Time
}

type updateProgressRenderer struct {
	writer     io.Writer
	rows       []updateProgressRow
	frameIndex int
	inPlace    bool
	nowFunc    func() time.Time
}

func newUpdateProgressRenderer(backends []toolBackend, writer io.Writer, inPlace bool) *updateProgressRenderer {
	nowFunc := time.Now
	rows := make([]updateProgressRow, 0, len(backends))
	for _, backend := range backends {
		rows = append(rows, updateProgressRow{toolName: backend.Name, startedAt: nowFunc()})
	}

	if writer == nil {
		writer = io.Discard
	}

	return &updateProgressRenderer{
		writer:  writer,
		rows:    rows,
		inPlace: inPlace,
		nowFunc: nowFunc,
	}
}

func (r *updateProgressRenderer) Start() {
	for idx := range r.rows {
		fmt.Fprintln(r.writer, r.lineForRow(idx))
	}
}

func (r *updateProgressRenderer) AdvanceFrame() {
	if len(loaderFrames) == 0 {
		return
	}

	r.frameIndex = (r.frameIndex + 1) % len(loaderFrames)
	if !r.inPlace {
		return
	}

	r.redrawAllRows()
}

func (r *updateProgressRenderer) MarkDone(index, packages int) {
	if index < 0 || index >= len(r.rows) {
		return
	}

	r.rows[index].done = true
	r.rows[index].packageCount = packages

	if r.inPlace {
		r.redrawAllRows()
		return
	}

	fmt.Fprintln(r.writer, r.lineForRow(index))
}

func (r *updateProgressRenderer) lineForRow(index int) string {
	if index < 0 || index >= len(r.rows) {
		return ""
	}

	row := r.rows[index]
	if row.done {
		return colorizedDoneLoaderLine(row.toolName, row.packageCount)
	}

	if len(loaderFrames) == 0 {
		return colorizedCheckingLoaderLine("", row.toolName)
	}

	frame := loaderFrames[r.frameIndex]
	if frame == "ooO" {
		elapsed := r.nowFunc().Sub(row.startedAt)
		if elapsed < loaderThirdFrameDelay {
			frame = "oOo"
		}
	}

	return colorizedCheckingLoaderLine(frame, row.toolName)
}

func (r *updateProgressRenderer) redrawAllRows() {
	if len(r.rows) == 0 {
		return
	}

	fmt.Fprintf(r.writer, "\033[%dA", len(r.rows))
	for idx := range r.rows {
		fmt.Fprintf(r.writer, "\r\033[2K%s\n", r.lineForRow(idx))
	}
}

func formatCheckingLoaderLine(frame, toolName string) string {
	frame = strings.TrimSpace(frame)
	if frame == "" {
		frame = "Ooo"
	}

	return fmt.Sprintf("%s [%s] checking...", frame, toolName)
}

func formatDoneLoaderLine(toolName string, packages int) string {
	return fmt.Sprintf("[%s] %d found", toolName, packages)
}

func colorizedCheckingLoaderLine(frame, toolName string) string {
	plainFrame := strings.TrimSpace(frame)
	if plainFrame == "" {
		plainFrame = "Ooo"
	}

	return fmt.Sprintf("%s %s %s",
		colorize(plainFrame, ansiBoldCyan),
		colorize("["+toolName+"]", ansiBlue),
		colorize("checking...", ansiYellow),
	)
}

func colorizedDoneLoaderLine(toolName string, packages int) string {
	return fmt.Sprintf("%s %s",
		colorize("["+toolName+"]", ansiBlue),
		colorize(fmt.Sprintf("%d found", packages), ansiGreen),
	)
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

func dnf5Backend() toolBackend {
	return toolBackend{
		Name: "dnf5",
		ListOutdated: func() ([]updatePackage, string, error) {
			packages, output, err := dnf5.ListOutdated()
			return mapDnf5OutdatedPackages(packages), output, err
		},
		UpdatePackage: func(packages []updatePackage) []packageUpdateResult {
			results := dnf5.UpdatePackages(mapToDnf5OutdatedPackages(packages))
			return mapDnf5UpdateResults(results)
		},
	}
}

func printUpdatePlanSections(plans []toolUpdatePlan) {
	fmt.Println(colorize("\nUpdate plan:", ansiBoldCyan))

	for _, plan := range plans {
		fmt.Printf("\n%s\n", colorize(fmt.Sprintf("[%s]", plan.ToolName), ansiBlue))
		if plan.CollectError != nil {
			fmt.Printf("%s\n", colorize(fmt.Sprintf("Could not collect outdated packages: %v", plan.CollectError), ansiRed))
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
				summary.UpdatedNames = append(summary.UpdatedNames, valueOrUnknown(result.Package.Name))
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
	fmt.Println(colorize("\nUpdate results:", ansiBoldCyan))

	totalUpdated := 0
	totalFailed := 0

	for _, summary := range summaries {
		totalUpdated += summary.UpdatedPackages
		totalFailed += summary.FailedPackages

		fmt.Printf("\n%s\n", colorize(fmt.Sprintf("[%s]", summary.ToolName), ansiBlue))
		fmt.Printf("Result: %s\n", colorizedStatus(summary.Status))
		fmt.Printf("Updated: %s\n", colorizedCount(summary.UpdatedPackages, ansiGreen))
		fmt.Printf("Failed: %s\n", colorizedCount(summary.FailedPackages, ansiRed))

		if len(summary.FailureReasons) > 0 {
			fmt.Println(colorize("Failure reasons:", ansiRed))
			for _, reason := range summary.FailureReasons {
				fmt.Printf("- %s\n", reason)
			}
		}

		fmt.Println(colorize("Output:", ansiBlue))
		if strings.TrimSpace(summary.Output) == "" {
			fmt.Println("(no output captured)")
			continue
		}

		printIndented(summary.Output, "  ")
	}

	fmt.Println()
	fmt.Print(framedBlock(overallSummaryDisplayLines(summaries, totalUpdated, totalFailed, summaryContentWidth())))
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

func mapDnf5OutdatedPackages(packages []dnf5.OutdatedPackage) []updatePackage {
	out := make([]updatePackage, 0, len(packages))
	for _, pkg := range packages {
		out = append(out, updatePackage{
			Name:      dnf5PackageSpec(pkg.Name, pkg.Arch),
			Requested: pkg.Repository,
			Current:   pkg.Current,
			Latest:    pkg.Latest,
		})
	}

	return out
}

func mapToDnf5OutdatedPackages(packages []updatePackage) []dnf5.OutdatedPackage {
	out := make([]dnf5.OutdatedPackage, 0, len(packages))
	for _, pkg := range packages {
		out = append(out, dnf5.OutdatedPackage{
			Name:       pkg.Name,
			Current:    pkg.Current,
			Latest:     pkg.Latest,
			Repository: pkg.Requested,
		})
	}

	return out
}

func mapDnf5UpdateResults(results []dnf5.PackageUpdateResult) []packageUpdateResult {
	out := make([]packageUpdateResult, 0, len(results))
	for _, result := range results {
		out = append(out, packageUpdateResult{
			Package: updatePackage{
				Name:      dnf5PackageSpec(result.Package.Name, result.Package.Arch),
				Requested: result.Package.Repository,
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

func dnf5PackageSpec(name, arch string) string {
	name = strings.TrimSpace(name)
	arch = strings.TrimSpace(arch)
	if arch == "" {
		return name
	}

	return fmt.Sprintf("%s.%s", name, arch)
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

func overallSummaryLines(summaries []toolUpdateSummary, totalUpdated, totalFailed int) []string {
	lines := []string{
		fmt.Sprintf("Updated: %d", totalUpdated),
		fmt.Sprintf("Failed: %d", totalFailed),
		"",
		"Updated packages by tool:",
	}

	for _, summary := range summaries {
		lines = append(lines, fmt.Sprintf("%s: %s", summary.ToolName, formatUpdatedNames(summary.UpdatedNames)))
	}

	return lines
}

func overallSummaryDisplayLines(summaries []toolUpdateSummary, totalUpdated, totalFailed, maxContentWidth int) []string {
	lines := []string{
		fmt.Sprintf("%s: %s", colorize("Updated", ansiBoldCyan), colorizedCount(totalUpdated, ansiGreen)),
		fmt.Sprintf("%s: %s", colorize("Failed", ansiBoldCyan), colorizedCount(totalFailed, ansiRed)),
		"",
	}

	for _, summary := range summaries {
		toolName := valueOrUnknown(summary.ToolName)
		normalizedNames := normalizedUpdatedNames(summary.UpdatedNames)
		wrappedLines := wrappedToolSummaryLines(toolName, normalizedNames, maxContentWidth)

		prefix := fmt.Sprintf("%s: ", toolName)
		continuationPrefix := strings.Repeat(" ", visibleLen(prefix))
		valueColor := ansiGreen
		if len(normalizedNames) == 0 {
			valueColor = ansiYellow
		}

		for idx, line := range wrappedLines {
			if idx == 0 {
				value := strings.TrimPrefix(line, prefix)
				lines = append(lines, fmt.Sprintf("%s: %s", colorize(toolName, ansiBlue), colorize(value, valueColor)))
				continue
			}

			value := strings.TrimPrefix(line, continuationPrefix)
			lines = append(lines, continuationPrefix+colorize(value, valueColor))
		}
	}

	return lines
}

func formatUpdatedNames(names []string) string {
	normalized := normalizedUpdatedNames(names)
	if len(normalized) == 0 {
		return "(none)"
	}

	return strings.Join(normalized, " ")
}

func normalizedUpdatedNames(names []string) []string {
	clean := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}

		seen[name] = struct{}{}
		clean = append(clean, name)
	}

	sort.Strings(clean)
	return clean
}

func wrappedToolSummaryLines(toolName string, names []string, maxContentWidth int) []string {
	prefix := fmt.Sprintf("%s: ", toolName)
	continuationPrefix := strings.Repeat(" ", visibleLen(prefix))
	prefixWidth := visibleLen(prefix)
	if maxContentWidth <= prefixWidth {
		maxContentWidth = prefixWidth + 1
	}

	if len(names) == 0 {
		return []string{prefix + "(none)"}
	}

	maxValueWidth := maxContentWidth - prefixWidth
	if maxValueWidth < 1 {
		maxValueWidth = 1
	}

	lines := make([]string, 0, len(names))
	currentPrefix := prefix
	currentWords := make([]string, 0, 4)
	currentValueWidth := 0

	flush := func() {
		if len(currentWords) == 0 {
			return
		}

		lines = append(lines, currentPrefix+strings.Join(currentWords, " "))
		currentPrefix = continuationPrefix
		currentWords = currentWords[:0]
		currentValueWidth = 0
	}

	for _, name := range names {
		for _, chunk := range splitByWidth(name, maxValueWidth) {
			chunkWidth := visibleLen(chunk)
			if len(currentWords) == 0 {
				currentWords = append(currentWords, chunk)
				currentValueWidth = chunkWidth
				continue
			}

			if currentValueWidth+1+chunkWidth <= maxValueWidth {
				currentWords = append(currentWords, chunk)
				currentValueWidth += 1 + chunkWidth
				continue
			}

			flush()
			currentWords = append(currentWords, chunk)
			currentValueWidth = chunkWidth
		}
	}

	flush()
	return lines
}

func splitByWidth(value string, maxWidth int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	if maxWidth <= 0 || visibleLen(value) <= maxWidth {
		return []string{value}
	}

	runes := []rune(value)
	parts := make([]string, 0, (len(runes)+maxWidth-1)/maxWidth)
	for start := 0; start < len(runes); start += maxWidth {
		end := start + maxWidth
		if end > len(runes) {
			end = len(runes)
		}

		parts = append(parts, string(runes[start:end]))
	}

	return parts
}

func summaryContentWidth() int {
	columns := terminalColumnsFunc()
	if columns <= frameHorizontalPadding {
		return defaultTerminalColumns - frameHorizontalPadding
	}

	return columns - frameHorizontalPadding
}

func detectTerminalColumns() int {
	envColumns := strings.TrimSpace(os.Getenv("COLUMNS"))
	if envColumns != "" {
		if parsed, err := strconv.Atoi(envColumns); err == nil && parsed > 0 {
			return parsed
		}
	}

	return defaultTerminalColumns
}

func framedBlock(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	maxWidth := 0
	for _, line := range lines {
		lineWidth := visibleLen(line)
		if lineWidth > maxWidth {
			maxWidth = lineWidth
		}
	}

	border := fmt.Sprintf("┌%s┐", strings.Repeat("─", maxWidth+2))

	var builder strings.Builder
	builder.WriteString(border)
	builder.WriteString("\n")
	for _, line := range lines {
		padding := strings.Repeat(" ", maxWidth-visibleLen(line))
		builder.WriteString("│ ")
		builder.WriteString(line)
		builder.WriteString(padding)
		builder.WriteString(" │\n")
	}
	builder.WriteString(fmt.Sprintf("└%s┘", strings.Repeat("─", maxWidth+2)))
	builder.WriteString("\n")

	return builder.String()
}

func visibleLen(text string) int {
	clean := ansiEscapePattern.ReplaceAllString(text, "")
	return utf8.RuneCountInString(clean)
}

func colorizedStatus(status string) string {
	description := describeStatus(status)

	switch status {
	case "updated", "up-to-date":
		return colorize(description, ansiGreen)
	case "partial", "cancelled":
		return colorize(description, ansiYellow)
	case "failed", "error":
		return colorize(description, ansiRed)
	default:
		return description
	}
}

func colorizedCount(value int, color string) string {
	text := fmt.Sprintf("%d", value)
	if value == 0 {
		return text
	}

	return colorize(text, color)
}

func colorize(text, color string) string {
	if !colorOutputEnabled || color == "" {
		return text
	}

	return color + text + ansiReset
}

func supportsInPlaceProgressOutput() bool {
	if strings.TrimSpace(os.Getenv("TERM")) == "" || os.Getenv("TERM") == "dumb" {
		return false
	}

	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

func supportsColorOutput() bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}

	if strings.TrimSpace(os.Getenv("TERM")) == "" || os.Getenv("TERM") == "dumb" {
		return false
	}

	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

func printIndented(text, indent string) {
	for _, line := range strings.Split(text, "\n") {
		fmt.Printf("%s%s\n", indent, line)
	}
}
