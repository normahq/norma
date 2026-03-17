package structured

import "errors"

var (
	ErrStructuredIOSchemaValidation = errors.New("structured I/O schema validation error")

	ErrStructuredInputSchemaValidation = errors.Join(
		errors.New("structured input schema validation error"),
		ErrStructuredIOSchemaValidation,
	)

	ErrStructuredOutputSchemaValidation = errors.Join(
		errors.New("structured output schema validation error"),
		ErrStructuredIOSchemaValidation,
	)
)
