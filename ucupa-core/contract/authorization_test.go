package contract

import (
	"context"
	"reflect"
	"testing"

	"ucupa/shared/values"
)

func TestAuthorizerSurface(t *testing.T) {
	authorizer := reflect.TypeOf((*Authorizer)(nil)).Elem()
	if authorizer.Kind() != reflect.Interface || authorizer.NumMethod() != 5 {
		t.Fatalf("Authorizer surface: type=%v methods=%d", authorizer, authorizer.NumMethod())
	}
	principal := reflect.TypeOf(values.PrincipalRef{})
	ctx := reflect.TypeOf((*context.Context)(nil)).Elem()
	requirements := reflect.TypeOf([]values.Requirement(nil))
	caller := reflect.TypeOf(values.CallerContext{})
	decision := reflect.TypeOf(values.AuthorizationUnavailable)
	view := reflect.TypeOf(values.AuthorizationView{})
	denial := reflect.TypeOf(values.AuthorizationDenialView{})
	want := map[string]reflect.Type{
		"AuthorizePrincipal": reflect.FuncOf([]reflect.Type{principal, requirements}, []reflect.Type{decision}, true),
		"AuthorizeView":      reflect.FuncOf([]reflect.Type{ctx, caller, requirements}, []reflect.Type{decision, view}, true),
		"ValidateView":       reflect.FuncOf([]reflect.Type{ctx, caller, view, requirements}, []reflect.Type{decision}, true),
		"CaptureDenialView":  reflect.FuncOf([]reflect.Type{ctx, caller, reflect.TypeOf(values.Requirement{})}, []reflect.Type{decision, denial}, false),
		"ValidateDenialView": reflect.FuncOf([]reflect.Type{ctx, caller, denial, reflect.TypeOf(values.Requirement{})}, []reflect.Type{decision}, false),
	}
	for name, signature := range want {
		method, exists := authorizer.MethodByName(name)
		if !exists || method.Type != signature {
			t.Fatalf("Authorizer method %s: exists=%t type=%v want=%v", name, exists, method.Type, signature)
		}
	}
}
