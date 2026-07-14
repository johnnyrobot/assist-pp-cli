// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var assistAgreementTypes = []string{"Prefix", "Department", "Major", "GeneralEducation"}

type advisorResolvedEntity struct {
	ID     int64           `json:"id"`
	Record json.RawMessage `json:"record"`
}

type advisorAgreementResolved struct {
	Sending      advisorResolvedEntity `json:"sending"`
	Receiving    advisorResolvedEntity `json:"receiving"`
	AcademicYear advisorResolvedEntity `json:"academic_year"`
}

type advisorPartnerVerification struct {
	Verified bool            `json:"verified"`
	Record   json.RawMessage `json:"record"`
	Response json.RawMessage `json:"response"`
}

type advisorAgreementResult struct {
	Type     string          `json:"type"`
	Response json.RawMessage `json:"response"`
}

type advisorAgreementsEnvelope struct {
	Source              string                     `json:"source"`
	Resolved            advisorAgreementResolved   `json:"resolved"`
	RequestedTypes      []string                   `json:"requested_types"`
	PartnerVerification advisorPartnerVerification `json:"partner_verification"`
	Agreements          []advisorAgreementResult   `json:"agreements"`
}

func newNovelAdvisorAgreementsCmd(flags *rootFlags) *cobra.Command {
	var flagSending string
	var flagReceiving string
	var flagYear string
	var flagTypes string

	// pp:data-source live
	cmd := &cobra.Command{
		Use:         "agreements",
		Short:       "Find official agreements from institution names, catalog year, and agreement types in one call.",
		Long:        "Use `advisor agreements` to discover published agreements between two institutions. Use `advisor course` when the question starts from one sending course.",
		Example:     "--sending Diablo Valley College --receiving UC Davis --year 2024-2025 --types Major,Department --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 && !flags.dryRun {
				return cmd.Help()
			}
			if len(args) != 0 {
				return usageErr(fmt.Errorf("advisor agreements accepts flags only; unexpected arguments: %s", strings.Join(args, " ")))
			}
			missing := make([]string, 0, 4)
			if strings.TrimSpace(flagSending) == "" {
				missing = append(missing, "--sending")
			}
			if strings.TrimSpace(flagReceiving) == "" {
				missing = append(missing, "--receiving")
			}
			if strings.TrimSpace(flagYear) == "" {
				missing = append(missing, "--year")
			}
			if strings.TrimSpace(flagTypes) == "" {
				missing = append(missing, "--types")
			}
			if len(missing) > 0 {
				return usageErr(fmt.Errorf("required flags missing: %s", strings.Join(missing, ", ")))
			}
			requestedTypes, err := parseAssistAgreementTypes(flagTypes)
			if err != nil {
				return usageErr(err)
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
			sending, err := referenceData.resolve(flagSending, flagYear)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			receiving, err := resolveAssistInstitution(referenceData.institutions, flagReceiving)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			partnerPath := "/api/institutions/{institutionId}/agreements"
			partnerPath = replacePathParam(partnerPath, "institutionId", strconv.FormatInt(sending.Institution.ID, 10))
			partnerResponse, err := c.Get(ctx, partnerPath, map[string]string{"asSendingOnly": "true"})
			if err != nil {
				return classifyAPIError(err, flags)
			}
			partnerRecord, verified, err := findAssistReceivingPartner(partnerResponse, receiving.ID, sending.AcademicYear.ID)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			if !verified {
				return notFoundErr(fmt.Errorf("receiving institution %d is not a published receiving partner of sending institution %d for academic year %d", receiving.ID, sending.Institution.ID, sending.AcademicYear.ID))
			}

			agreementResults := make([]advisorAgreementResult, 0, len(requestedTypes))
			for _, agreementType := range requestedTypes {
				path := publicAgreementListPath(
					strconv.FormatInt(receiving.ID, 10),
					strconv.FormatInt(sending.Institution.ID, 10),
					strconv.FormatInt(sending.AcademicYear.ID, 10),
					agreementType,
				)
				response, getErr := c.Get(ctx, path, nil)
				if getErr != nil {
					return classifyAPIError(getErr, flags)
				}
				if !json.Valid(response) {
					return classifyAPIError(fmt.Errorf("ASSIST agreement response for type %s was not valid JSON", agreementType), flags)
				}
				agreementResults = append(agreementResults, advisorAgreementResult{Type: agreementType, Response: cloneRaw(response)})
			}

			out := advisorAgreementsEnvelope{
				Source: "live",
				Resolved: advisorAgreementResolved{
					Sending:      advisorResolvedEntity{ID: sending.Institution.ID, Record: sending.Institution.Record},
					Receiving:    advisorResolvedEntity{ID: receiving.ID, Record: receiving.Record},
					AcademicYear: advisorResolvedEntity{ID: sending.AcademicYear.ID, Record: sending.AcademicYear.Record},
				},
				RequestedTypes: requestedTypes,
				PartnerVerification: advisorPartnerVerification{
					Verified: true,
					Record:   partnerRecord,
					Response: cloneRaw(partnerResponse),
				},
				Agreements: agreementResults,
			}
			return printJSONFilteredWithMeta(cmd.OutOrStdout(), out, flags, map[string]any{"source": "live"})
		},
	}
	cmd.Flags().StringVar(&flagSending, "sending", "", "Sending institution name or code")
	cmd.Flags().StringVar(&flagReceiving, "receiving", "", "Receiving institution name or code")
	cmd.Flags().StringVar(&flagYear, "year", "", "Academic year ID, code, description, or date label")
	cmd.Flags().StringVar(&flagTypes, "types", "", "Comma-separated agreement types: Prefix, Department, Major, GeneralEducation")
	return cmd
}

func parseAssistAgreementTypes(value string) ([]string, error) {
	allowed := make(map[string]string, len(assistAgreementTypes))
	for _, agreementType := range assistAgreementTypes {
		allowed[strings.ToLower(agreementType)] = agreementType
	}
	seen := make(map[string]bool, len(assistAgreementTypes))
	result := make([]string, 0, len(assistAgreementTypes))
	for _, raw := range strings.Split(value, ",") {
		candidate := strings.TrimSpace(raw)
		canonical, ok := allowed[strings.ToLower(candidate)]
		if !ok || candidate == "" {
			return nil, fmt.Errorf("invalid agreement type %q; must be one of %s", candidate, strings.Join(assistAgreementTypes, ", "))
		}
		if !seen[canonical] {
			seen[canonical] = true
			result = append(result, canonical)
		}
	}
	return result, nil
}

func findAssistReceivingPartner(response json.RawMessage, receivingInstitutionID, academicYearID int64) (json.RawMessage, bool, error) {
	records, err := assistRecordList(response)
	if err != nil {
		return nil, false, fmt.Errorf("decode ASSIST published-partner response: %w", err)
	}
	for _, record := range records {
		obj, objectErr := assistObject(record)
		if objectErr != nil {
			return nil, false, fmt.Errorf("decode ASSIST published-partner record: %w", objectErr)
		}
		institutionID, ok := assistObjectInt(obj, "institutionParentId", "institutionId", "receivingInstitutionId", "id")
		if !ok {
			if nested, nestedOK := assistLookup(obj, "institution", "receivingInstitution"); nestedOK {
				if nestedObj, nestedErr := assistObject(nested); nestedErr == nil {
					institutionID, ok = assistObjectInt(nestedObj, "id", "institutionId")
				}
			}
		}
		if !ok || institutionID != receivingInstitutionID {
			continue
		}
		for _, key := range []string{"receivingYearIds", "receivingAcademicYearIds", "academicYearIds", "receivingYears"} {
			if rawYears, yearsOK := assistLookup(obj, key); yearsOK && assistContainsID(assistNumericValues(rawYears), academicYearID) {
				return cloneRaw(record), true, nil
			}
		}
	}
	return nil, false, nil
}
