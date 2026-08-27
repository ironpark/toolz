package main

import "errors"

// validationRecord is the machine-readable form of a document validation
// failure. The human error remains a single, deliberately stable string; JSON
// callers get the location and rule without having to scrape that string.
type validationRecord struct {
	Rule    string
	Section string
	Phase   *int
	Line    int
	Phases  []int
	Detail  string
}

type validationFailure struct {
	records []validationRecord
	message string
}

func (failure *validationFailure) Error() string {
	return failure.message
}

func newValidationFailure(record validationRecord, message string) error {
	return &validationFailure{records: []validationRecord{record}, message: message}
}

func newValidationFailures(records []validationRecord, message string) error {
	return &validationFailure{records: records, message: message}
}

func validationRecords(err error) []validationRecord {
	var failure *validationFailure
	if !errors.As(err, &failure) {
		return nil
	}
	return append([]validationRecord{}, failure.records...)
}

func wrapValidationError(err error, rule, section string) error {
	if err == nil || len(validationRecords(err)) > 0 {
		return err
	}
	return newValidationFailure(validationRecord{Rule: rule, Section: section, Detail: err.Error()}, err.Error())
}

func validationIntPointer(value int) *int {
	return &value
}
