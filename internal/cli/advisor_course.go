// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

// pp:data-source live

type advisorCourseResolvedCourse struct {
	CourseIdentifierParentID int64           `json:"course_identifier_parent_id"`
	Prefix                   string          `json:"prefix,omitempty"`
	Number                   string          `json:"number,omitempty"`
	Title                    string          `json:"title,omitempty"`
	TermID                   *int64          `json:"term_id,omitempty"`
	Record                   json.RawMessage `json:"record"`
}

type advisorCourseResolved struct {
	Sending      advisorResolvedEntity       `json:"sending"`
	Receiving    advisorResolvedEntity       `json:"receiving"`
	AcademicYear advisorResolvedEntity       `json:"academic_year"`
	Course       advisorCourseResolvedCourse `json:"course"`
}

type advisorCourseArticulationSection struct {
	Result   json.RawMessage `json:"result"`
	Response json.RawMessage `json:"response"`
}

type advisorCourseEnvelope struct {
	Source                 string                           `json:"source"`
	Resolved               advisorCourseResolved            `json:"resolved"`
	CourseSearch           json.RawMessage                  `json:"course_search"`
	Transferability        json.RawMessage                  `json:"transferability"`
	DepartmentArticulation advisorCourseArticulationSection `json:"department_articulation"`
	MajorArticulation      advisorCourseArticulationSection `json:"major_articulation"`
}

func newNovelAdvisorCourseCmd(flags *rootFlags) *cobra.Command {
	var flagInstitution string
	var flagReceiving string
	var flagYear string

	cmd := &cobra.Command{
		Use:         "course <course-query>",
		Short:       "See a course's official transferability plus department and major articulation matches together.",
		Long:        "Use advisor course for a course-centered transferability and articulation answer. Use advisor agreements to enumerate agreements without starting from a course.",
		Example:     "  assist-pp-cli advisor course 'CHEM 25' --institution 'Diablo Valley College' --receiving 'UC Davis' --year 2024-2025 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 && !flags.dryRun {
				return cmd.Help()
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				return usageErr(fmt.Errorf("advisor course requires exactly one non-empty course query\nUsage: %s <course-query> --institution <sending-name> --receiving <target-name> --year <display-or-code>", cmd.CommandPath()))
			}
			missing := make([]string, 0, 3)
			if strings.TrimSpace(flagInstitution) == "" {
				missing = append(missing, "--institution")
			}
			if strings.TrimSpace(flagReceiving) == "" {
				missing = append(missing, "--receiving")
			}
			if strings.TrimSpace(flagYear) == "" {
				missing = append(missing, "--year")
			}
			if len(missing) > 0 {
				return usageErr(fmt.Errorf("required flags missing: %s", strings.Join(missing, ", ")))
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
			sending, err := referenceData.resolve(flagInstitution, flagYear)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			receiving, err := resolveAssistInstitution(referenceData.institutions, flagReceiving)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			coursePath := "/Courses/api/Institution/{institutionId}/Year/{academicYearId}"
			coursePath = replacePathParam(coursePath, "institutionId", strconv.FormatInt(sending.Institution.ID, 10))
			coursePath = replacePathParam(coursePath, "academicYearId", strconv.FormatInt(sending.AcademicYear.ID, 10))
			courseResponse, err := c.Get(ctx, coursePath, map[string]string{"filter": args[0]})
			if err != nil {
				return classifyAPIError(err, flags)
			}
			course, err := resolveAdvisorCourse(courseResponse, args[0])
			if err != nil {
				return classifyAPIError(err, flags)
			}

			courseID := strconv.FormatInt(course.CourseIdentifierParentID, 10)
			sendingID := strconv.FormatInt(sending.Institution.ID, 10)
			receivingID := strconv.FormatInt(receiving.ID, 10)
			yearID := strconv.FormatInt(sending.AcademicYear.ID, 10)

			transferabilityPath := "/Transferability/api/Courses/{courseIdentifierParentId}/Institution/{institutionId}/Year/{academicYearId}"
			transferabilityPath = replacePathParam(transferabilityPath, "courseIdentifierParentId", courseID)
			transferabilityPath = replacePathParam(transferabilityPath, "institutionId", sendingID)
			transferabilityPath = replacePathParam(transferabilityPath, "academicYearId", yearID)
			transferabilityResponse, err := c.Get(ctx, transferabilityPath, nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			if !json.Valid(transferabilityResponse) {
				return classifyAPIError(fmt.Errorf("ASSIST transferability response was not valid JSON"), flags)
			}

			articulationParams := map[string]string{}
			if course.TermID != nil {
				articulationParams["termId"] = strconv.FormatInt(*course.TermID, 10)
			}
			departmentPath := advisorCourseArticulationPath("Department", sendingID, receivingID, courseID, yearID)
			departmentResponse, err := c.Get(ctx, departmentPath, articulationParams)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			departmentSection, err := decodeAdvisorCourseArticulation(departmentResponse, "department")
			if err != nil {
				return classifyAPIError(err, flags)
			}

			majorPath := advisorCourseArticulationPath("Major", sendingID, receivingID, courseID, yearID)
			majorResponse, err := c.Get(ctx, majorPath, articulationParams)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			majorSection, err := decodeAdvisorCourseArticulation(majorResponse, "major")
			if err != nil {
				return classifyAPIError(err, flags)
			}

			out := advisorCourseEnvelope{
				Source: "live",
				Resolved: advisorCourseResolved{
					Sending:      advisorResolvedEntity{ID: sending.Institution.ID, Record: sending.Institution.Record},
					Receiving:    advisorResolvedEntity{ID: receiving.ID, Record: receiving.Record},
					AcademicYear: advisorResolvedEntity{ID: sending.AcademicYear.ID, Record: sending.AcademicYear.Record},
					Course:       course,
				},
				CourseSearch:           cloneRaw(courseResponse),
				Transferability:        cloneRaw(transferabilityResponse),
				DepartmentArticulation: departmentSection,
				MajorArticulation:      majorSection,
			}
			return printJSONFilteredWithMeta(cmd.OutOrStdout(), out, flags, map[string]any{"source": "live"})
		},
	}
	cmd.Flags().StringVar(&flagInstitution, "institution", "", "Sending institution name or code")
	cmd.Flags().StringVar(&flagReceiving, "receiving", "", "Receiving institution name or code")
	cmd.Flags().StringVar(&flagYear, "year", "", "Academic year ID, code, description, or date label")
	return cmd
}

func advisorCourseArticulationPath(kind, sendingID, receivingID, courseID, yearID string) string {
	path := "/articulation/api/Agreements/{kind}/from/{sendingInstitutionParentId}/to/{receivingInstitutionParentId}/sending/course/{courseIdentifierParentId}/for/{academicYearId}"
	path = replacePathParam(path, "kind", kind)
	path = replacePathParam(path, "sendingInstitutionParentId", sendingID)
	path = replacePathParam(path, "receivingInstitutionParentId", receivingID)
	path = replacePathParam(path, "courseIdentifierParentId", courseID)
	return replacePathParam(path, "academicYearId", yearID)
}

func decodeAdvisorCourseArticulation(response json.RawMessage, kind string) (advisorCourseArticulationSection, error) {
	if !json.Valid(response) {
		return advisorCourseArticulationSection{}, fmt.Errorf("ASSIST %s articulation response was not valid JSON", kind)
	}
	obj, err := assistObject(response)
	if err != nil {
		return advisorCourseArticulationSection{}, fmt.Errorf("decode ASSIST %s articulation response: %w", kind, err)
	}
	result, ok := assistLookup(obj, "result")
	if !ok {
		return advisorCourseArticulationSection{}, fmt.Errorf("ASSIST %s articulation response is missing the result envelope", kind)
	}
	return advisorCourseArticulationSection{Result: cloneRaw(result), Response: cloneRaw(response)}, nil
}

type advisorCourseCandidate struct {
	course advisorCourseResolvedCourse
	score  int
}

func resolveAdvisorCourse(response json.RawMessage, query string) (advisorCourseResolvedCourse, error) {
	records, err := advisorCourseRecords(response)
	if err != nil {
		return advisorCourseResolvedCourse{}, fmt.Errorf("decode ASSIST course-search response: %w", err)
	}
	queryWords := normalizeAssistLabel(query)
	queryCompact := compactAdvisorCourseLabel(query)
	if queryCompact == "" {
		return advisorCourseResolvedCourse{}, usageErr(fmt.Errorf("course query must contain at least one letter or number"))
	}

	candidates := make([]advisorCourseCandidate, 0)
	for _, record := range records {
		obj, objectErr := assistObject(record)
		if objectErr != nil {
			return advisorCourseResolvedCourse{}, fmt.Errorf("decode ASSIST course record: %w", objectErr)
		}
		courseID, ok := assistObjectInt(obj, "courseIdentifierParentId")
		if !ok || courseID <= 0 {
			continue
		}
		prefix, _ := assistObjectString(obj, "prefix", "coursePrefix")
		number, _ := assistObjectString(obj, "courseNumber", "number")
		title, _ := assistObjectString(obj, "courseTitle", "title")
		score := advisorCourseMatchScore(queryWords, queryCompact, prefix, number, title)
		if score == 0 {
			continue
		}
		var termID *int64
		if value, found := assistObjectInt(obj, "publishedCourseIdentifierYearTermId", "termId", "yearTermId", "beginTermId"); found && value > 0 {
			termID = &value
		}
		candidates = append(candidates, advisorCourseCandidate{
			course: advisorCourseResolvedCourse{
				CourseIdentifierParentID: courseID,
				Prefix:                   prefix,
				Number:                   number,
				Title:                    title,
				TermID:                   termID,
				Record:                   cloneRaw(record),
			},
			score: score,
		})
	}

	if len(candidates) == 0 {
		return advisorCourseResolvedCourse{}, notFoundErr(fmt.Errorf("course %q was not found as an exact prefix-number or title match in the sending institution's live ASSIST catalog", query))
	}
	bestScore := 0
	for _, candidate := range candidates {
		if candidate.score > bestScore {
			bestScore = candidate.score
		}
	}
	byID := make(map[int64]advisorCourseResolvedCourse)
	for _, candidate := range candidates {
		if candidate.score != bestScore {
			continue
		}
		existing, exists := byID[candidate.course.CourseIdentifierParentID]
		if !exists || string(candidate.course.Record) < string(existing.Record) {
			byID[candidate.course.CourseIdentifierParentID] = candidate.course
		}
	}
	ids := make([]int64, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) != 1 {
		return advisorCourseResolvedCourse{}, usageErr(fmt.Errorf("course %q is ambiguous; matching ASSIST courseIdentifierParentIds: %s", query, joinAssistIDs(ids)))
	}
	return byID[ids[0]], nil
}

func advisorCourseMatchScore(queryWords, queryCompact, prefix, number, title string) int {
	codeWords := normalizeAssistLabel(strings.TrimSpace(prefix + " " + number))
	codeCompact := compactAdvisorCourseLabel(prefix + number)
	titleWords := normalizeAssistLabel(title)
	fullWords := normalizeAssistLabel(strings.TrimSpace(prefix + " " + number + " " + title))
	switch {
	case fullWords != "" && queryWords == fullWords:
		return 3
	case codeWords != "" && (queryWords == codeWords || queryCompact == codeCompact):
		return 2
	case titleWords != "" && queryWords == titleWords:
		return 1
	default:
		return 0
	}
}

func compactAdvisorCourseLabel(value string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func advisorCourseRecords(raw json.RawMessage) ([]json.RawMessage, error) {
	if records, err := assistRecordList(raw); err == nil {
		return records, nil
	}
	trimmed := json.RawMessage(strings.TrimSpace(string(raw)))
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("response does not contain a course collection")
	}
	obj, err := assistObject(trimmed)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"result", "results", "data", "items", "courses", "value"} {
		if nested, ok := assistLookup(obj, key); ok && string(nested) != "null" {
			if records, nestedErr := advisorCourseRecords(nested); nestedErr == nil {
				return records, nil
			}
		}
	}
	return nil, fmt.Errorf("response does not contain a course collection")
}
