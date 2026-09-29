package nvi

import "context"

type registrationCategoryKey struct{}

// RegistrationCategory identifies the category of a registration request for
// transport instrumentation without reading or retaining its FHIR payload.
func RegistrationCategory(ctx context.Context) string {
	category, _ := ctx.Value(registrationCategoryKey{}).(string)
	return category
}
