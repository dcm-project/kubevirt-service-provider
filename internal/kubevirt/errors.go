package kubevirt

import (
	"errors"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
)

// problemError creates a server.Error with the standard "about:blank" type.
func problemError(status int, title, detail string) server.Error {
	typ := "about:blank"
	return server.Error{
		Title:  title,
		Type:   typ,
		Status: &status,
		Detail: &detail,
	}
}

// classifyKubernetesError extracts status code and title from a Kubernetes error.
// The fallbackDetail is used when the original error should not be exposed to clients.
func classifyKubernetesError(err error, fallbackDetail string) (server.Error, int) {
	var statusErr *apierrors.StatusError
	if !errors.As(err, &statusErr) {
		return problemError(http.StatusInternalServerError, "Internal Server Error", err.Error()), http.StatusInternalServerError
	}

	switch statusErr.ErrStatus.Code {
	case http.StatusConflict:
		return problemError(http.StatusConflict, "Conflict", statusErr.ErrStatus.Message), http.StatusConflict
	case http.StatusUnprocessableEntity:
		return problemError(http.StatusUnprocessableEntity, "Validation Error", statusErr.ErrStatus.Message), http.StatusUnprocessableEntity
	case http.StatusBadRequest:
		return problemError(http.StatusBadRequest, "Bad Request", statusErr.ErrStatus.Message), http.StatusBadRequest
	case http.StatusNotFound:
		return problemError(http.StatusNotFound, "Not Found", statusErr.ErrStatus.Message), http.StatusNotFound
	default:
		return problemError(http.StatusInternalServerError, "Internal Server Error", fallbackDetail), http.StatusInternalServerError
	}
}

// InternalServerError returns a problem+json error body and 500 status code.
func InternalServerError(detail string) (server.Error, int) {
	return problemError(http.StatusInternalServerError, "Internal Server Error", detail), http.StatusInternalServerError
}

// ValidationError returns a problem+json error body and 400 status code.
func ValidationError(detail string) (server.Error, int) {
	return problemError(http.StatusBadRequest, "Validation Error", detail), http.StatusBadRequest
}

// IsAlreadyExistsError checks if the error indicates a resource already exists.
func IsAlreadyExistsError(err error) bool {
	return apierrors.IsAlreadyExists(err)
}

// IsNotFoundError checks if the error indicates a resource was not found.
func IsNotFoundError(err error) bool {
	return apierrors.IsNotFound(err)
}

// IsInvalidError checks if the error indicates invalid input.
func IsInvalidError(err error) bool {
	return apierrors.IsInvalid(err)
}

// MapKubernetesError maps Kubernetes API errors to CreateVM responses.
func MapKubernetesError(err error) server.CreateVMResponseObject {
	if err == nil {
		return nil
	}
	body, statusCode := classifyKubernetesError(err, "Failed to create virtual machine")
	return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
		Body:       body,
		StatusCode: statusCode,
	}
}

// MapKubernetesErrorForDelete maps Kubernetes API errors to DeleteVM responses.
func MapKubernetesErrorForDelete(err error) server.DeleteVMResponseObject {
	if err == nil {
		return nil
	}
	body, statusCode := classifyKubernetesError(err, "Failed to delete virtual machine")
	if statusCode == http.StatusNotFound {
		return server.DeleteVM404ApplicationProblemPlusJSONResponse(body)
	}
	return server.DeleteVMdefaultApplicationProblemPlusJSONResponse{
		Body:       body,
		StatusCode: statusCode,
	}
}

// MapKubernetesErrorForGet maps Kubernetes API errors to GetVM responses.
func MapKubernetesErrorForGet(err error) server.GetVMResponseObject {
	if err == nil {
		return nil
	}
	body, statusCode := classifyKubernetesError(err, "Failed to retrieve virtual machine")
	if statusCode == http.StatusNotFound {
		return server.GetVM404ApplicationProblemPlusJSONResponse(body)
	}
	return server.GetVMdefaultApplicationProblemPlusJSONResponse{
		Body:       body,
		StatusCode: statusCode,
	}
}

// MapKubernetesErrorForList maps Kubernetes API errors to ListVMs responses.
func MapKubernetesErrorForList(err error) server.ListVMsResponseObject {
	if err == nil {
		return nil
	}
	body, statusCode := classifyKubernetesError(err, "Failed to list virtual machines")
	return &server.ListVMsdefaultApplicationProblemPlusJSONResponse{
		Body:       body,
		StatusCode: statusCode,
	}
}
