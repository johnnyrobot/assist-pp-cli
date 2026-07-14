// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelResolveInstitutionCmd(flags *rootFlags) *cobra.Command {
	var flagYear string

	// pp:data-source live
	cmd := &cobra.Command{
		Use:         "institution <name>",
		Short:       "Turn recognizable institution and academic-year names into exact ASSIST identifiers.",
		Long:        "Use `resolve institution` when you need explicit institution and academic-year identifiers. Use `advisor agreements` when you want the complete sending-to-receiving agreement lookup instead.",
		Example:     "Diablo Valley College --year 2024-2025 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !cmd.Flags().Changed("year") && !flags.dryRun {
				return cmd.Help()
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				return usageErr(fmt.Errorf("exactly one institution name or code is required\nUsage: %s <name> --year <display-or-code>", cmd.CommandPath()))
			}
			if strings.TrimSpace(flagYear) == "" {
				return usageErr(fmt.Errorf("required flag missing: --year\nUsage: %s <name> --year <display-or-code>", cmd.CommandPath()))
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return nil
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			referenceData, err := fetchAssistResolutionData(ctx, c)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			resolved, err := referenceData.resolve(args[0], flagYear)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			result := map[string]any{
				"source":           "live",
				"institution_id":   resolved.Institution.ID,
				"academic_year_id": resolved.AcademicYear.ID,
				"institution":      resolved.Institution.Record,
				"academic_year":    resolved.AcademicYear.Record,
			}
			return printJSONFilteredWithMeta(cmd.OutOrStdout(), result, flags, map[string]any{"source": "live"})
		},
	}
	cmd.Flags().StringVar(&flagYear, "year", "", "Academic year ID, code, description, or date label")
	return cmd
}
