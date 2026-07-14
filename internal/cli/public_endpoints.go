// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
// Public-site endpoint commands. These routes mirror assist.org's anonymous web app.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func runPublicGET(cmd *cobra.Command, flags *rootFlags, path string, params map[string]string) error {
	c, err := flags.newClient()
	if err != nil {
		return err
	}
	data, err := c.Get(cmd.Context(), path, params)
	if err != nil {
		return classifyAPIError(err, flags)
	}
	return printOutputWithFlagsMeta(cmd.OutOrStdout(), data, flags, map[string]any{
		"source":   "live",
		"endpoint": path,
		"auth":     "anonymous",
	})
}

func requirePublicArgs(cmd *cobra.Command, args []string, count int) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	if len(args) != count {
		return usageErr(fmt.Errorf("expected %d arguments, received %d", count, len(args)))
	}
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			return usageErr(fmt.Errorf("arguments must not be empty"))
		}
	}
	return nil
}

func newPublicSettingsCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "settings",
		Short:       "Show public ASSIST application limits and version settings.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"pp:endpoint": "settings.get", "pp:method": "GET", "pp:path": "/api/appsettings", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPublicGET(cmd, flags, "/api/appsettings", nil)
		},
	}
}

func newPublicAgreementCategoriesCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "categories <receivingInstitutionId> <sendingInstitutionId> <academicYearId>",
		Short:       "List public agreement categories available for an institution pair and year.",
		Example:     "  assist-pp-cli agreements categories 7 110 74 --json",
		Annotations: map[string]string{"pp:endpoint": "agreements.categories", "pp:method": "GET", "pp:path": "/api/agreements/categories", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePublicArgs(cmd, args, 3); err != nil {
				return err
			}
			return runPublicGET(cmd, flags, "/api/agreements/categories", map[string]string{
				"receivingInstitutionId": args[0],
				"sendingInstitutionId":   args[1],
				"academicYearId":         args[2],
			})
		},
	}
}

func newPublicTransferabilityYearsCmd(flags *rootFlags) *cobra.Command {
	var listType string
	cmd := &cobra.Command{
		Use:         "years <institutionId>",
		Short:       "List public academic years available for an institution's transferability lists.",
		Example:     "  assist-pp-cli transferability years 110 --json\n  assist-pp-cli transferability years 110 --list-type UCTEL --json",
		Annotations: map[string]string{"pp:endpoint": "transferability.years", "pp:method": "GET", "pp:path": "/api/institutions/{institutionId}/transferability/availableAcademicYears", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePublicArgs(cmd, args, 1); err != nil {
				return err
			}
			path := "/api/institutions/" + args[0] + "/transferability/availableAcademicYears"
			if strings.TrimSpace(listType) != "" {
				path = "/api/institutions/" + args[0] + "/transferability/" + listType + "/availableAcademicYears"
			}
			return runPublicGET(cmd, flags, path, nil)
		},
	}
	cmd.Flags().StringVar(&listType, "list-type", "", "Optional list type such as UCTEL, UCTCA, IGETC, CSUGE, CSUAI, or CALGETC")
	return cmd
}

func newPublicTransferabilityCategoriesCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "categories <institutionId> <academicYearId> <listType>",
		Short:       "List public grouping categories for a transferability list.",
		Example:     "  assist-pp-cli transferability categories 110 74 UCTEL --json",
		Annotations: map[string]string{"pp:endpoint": "transferability.categories", "pp:method": "GET", "pp:path": "/api/transferability/categories", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePublicArgs(cmd, args, 3); err != nil {
				return err
			}
			return runPublicGET(cmd, flags, "/api/transferability/categories", map[string]string{
				"institutionId":  args[0],
				"academicYearId": args[1],
				"listType":       args[2],
			})
		},
	}
}

func newPublicTransferabilityCoursesCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "courses <institutionId> <academicYearId> <listType>",
		Short:       "Fetch the public course data for a transferability list.",
		Example:     "  assist-pp-cli transferability courses 110 74 UCTEL --json",
		Annotations: map[string]string{"pp:endpoint": "transferability.courses", "pp:method": "GET", "pp:path": "/api/transferability/courses", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePublicArgs(cmd, args, 3); err != nil {
				return err
			}
			return runPublicGET(cmd, flags, "/api/transferability/courses", map[string]string{
				"institutionId":  args[0],
				"academicYearId": args[1],
				"listType":       args[2],
			})
		},
	}
}
