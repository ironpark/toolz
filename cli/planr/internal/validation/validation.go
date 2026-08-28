package validation

import "errors"

// Record is the machine-readable form of a document validation
// failure. The human error remains a single, deliberately stable string; JSON
// callers get the location and rule without having to scrape that string.
type Record struct {
	Rule    string
	Section string
	Phase   *int
	Line    int
	Phases  []int
	Detail  string
}

type failure struct {
	records []Record
	message string
}

func (failure *failure) Error() string {
	return failure.message
}

func NewFailure(record Record, message string) error {
	return &failure{records: []Record{record}, message: message}
}

func NewFailures(records []Record, message string) error {
	return &failure{records: records, message: message}
}

func Records(err error) []Record {
	var failure *failure
	if !errors.As(err, &failure) {
		return nil
	}
	return append([]Record{}, failure.records...)
}

func Wrap(err error, rule, section string) error {
	if err == nil || len(Records(err)) > 0 {
		return err
	}
	return NewFailure(Record{Rule: rule, Section: section, Detail: err.Error()}, err.Error())
}

func IntPointer(value int) *int {
	return &value
}
