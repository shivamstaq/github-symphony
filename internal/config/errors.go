package config

import "errors"

// WorkflowErrorKind identifies the category of workflow error.
type WorkflowErrorKind string

const (
	ErrMissingWorkflowFile WorkflowErrorKind = "missing_workflow_file"
	ErrWorkflowParseError  WorkflowErrorKind = "workflow_parse_error"
	ErrFrontMatterNotAMap  WorkflowErrorKind = "workflow_front_matter_not_a_map"
	ErrTemplateParseError  WorkflowErrorKind = "template_parse_error"
	ErrTemplateRenderError WorkflowErrorKind = "template_render_error"
)

// WorkflowError is a typed error for workflow loading failures.
type WorkflowError struct {
	Kind    WorkflowErrorKind
	Message string
	Cause   error
}

func (e *WorkflowError) Error() string {
	if e.Cause != nil {
		return string(e.Kind) + ": " + e.Message + ": " + e.Cause.Error()
	}
	return string(e.Kind) + ": " + e.Message
}

func (e *WorkflowError) Unwrap() error {
	return e.Cause
}

// AsWorkflowError is a convenience for errors.As with WorkflowError.
func AsWorkflowError(err error, target **WorkflowError) bool {
	return errors.As(err, target)
}
