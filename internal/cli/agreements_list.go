// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

func newAgreementsListCmd(flags *rootFlags) *cobra.Command {
	var agreementType string
	cmd := &cobra.Command{
		Use:         "list <receivingInstitutionId> <sendingInstitutionId> <academicYearId>",
		Short:       "List public agreements for an institution pair, year, and category.",
		Example:     "  assist-pp-cli agreements list 7 110 74 --types Department --json",
		Annotations: map[string]string{"pp:endpoint": "agreements.list", "pp:method": "GET", "pp:path": "/api/agreements", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePublicArgs(cmd, args, 3); err != nil {
				return err
			}
			if agreementType == "" {
				return usageErr(fmt.Errorf("required flag \"types\" not set"))
			}
			allowed := map[string]bool{"Prefix": true, "Department": true, "Major": true, "GeneralEducation": true}
			if !allowed[agreementType] {
				return usageErr(fmt.Errorf("invalid value %q for --types: must be Prefix, Department, Major, or GeneralEducation", agreementType))
			}
			return runPublicGET(cmd, flags, publicAgreementListPath(args[0], args[1], args[2], agreementType), nil)
		},
	}
	cmd.Flags().StringVar(&agreementType, "types", "", "Agreement category: Prefix, Department, Major, or GeneralEducation")
	return cmd
}

// ASSIST's public agreement endpoint is sensitive to the web application's
// query-parameter order. url.Values.Encode sorts keys and can return an empty
// report set, so preserve the browser route's receiving/sending/year/category
// sequence exactly.
func publicAgreementListPath(receivingID, sendingID, yearID, agreementType string) string {
	return "/api/agreements?receivingInstitutionId=" + url.QueryEscape(receivingID) +
		"&sendingInstitutionId=" + url.QueryEscape(sendingID) +
		"&academicYearId=" + url.QueryEscape(yearID) +
		"&categoryCode=" + url.QueryEscape(publicAgreementCategoryCode(agreementType))
}

func publicAgreementCategoryCode(value string) string {
	switch value {
	case "Prefix":
		return "prefix"
	case "Department":
		return "dept"
	case "Major":
		return "major"
	case "GeneralEducation":
		return "breadth"
	default:
		return value
	}
}
