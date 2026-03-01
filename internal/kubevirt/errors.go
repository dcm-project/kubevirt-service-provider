package kubevirt

import (
	"errors"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
)

// MapKubernetesError maps Kubernetes API errors to appropriate HTTP responses
func MapKubernetesError(err error) server.CreateVMResponseObject {
	if err == nil {
		return nil
	}

	var statusErr *apierrors.StatusError
	if !errors.As(err, &statusErr) {
		// Non-Kubernetes error, treat as internal server error
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		typ := "about:blank"
		detail := err.Error()
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}
	}

	switch statusErr.ErrStatus.Code {
	case http.StatusConflict:
		// 409 - Resource already exists
		status := http.StatusConflict
		title := "Conflict"
		typ := "about:blank"
		detail := statusErr.ErrStatus.Message
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}

	case http.StatusUnprocessableEntity:
		// 422 - Validation error
		status := http.StatusUnprocessableEntity
		title := "Validation Error"
		typ := "about:blank"
		detail := statusErr.ErrStatus.Message
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}

	case http.StatusBadRequest:
		// 400 - Bad request
		status := http.StatusBadRequest
		title := "Bad Request"
		typ := "about:blank"
		detail := statusErr.ErrStatus.Message
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}

	case http.StatusForbidden:
		// 403 - Forbidden, map to internal server error to avoid exposing auth details
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		typ := "about:blank"
		detail := "Failed to create virtual machine"
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}

	case http.StatusNotFound:
		// 404 - Namespace or resource not found, map to internal server error
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		typ := "about:blank"
		detail := "Failed to create virtual machine"
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}

	default:
		// Any other Kubernetes error, treat as internal server error
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		typ := "about:blank"
		detail := "Failed to create virtual machine"
		return &server.CreateVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}
	}
}

// InternalServerError returns a problem+json error body and 500 status code for internal server errors.
// Handlers can use this to build their default application/problem+json response.
func InternalServerError(detail string) (server.Error, int) {
	status := http.StatusInternalServerError
	title := "Internal Server Error"
	typ := "about:blank"
	return server.Error{
		Title:  title,
		Type:   typ,
		Status: &status,
		Detail: &detail,
	}, status
}

func ValidationError(detail string) (server.Error, int) {
	status := http.StatusBadRequest
	title := "Validation Error"
	typ := "about:blank"
	return server.Error{
		Title:  title,
		Type:   typ,
		Status: &status,
		Detail: &detail,
	}, status
}

// IsAlreadyExistsError checks if the error indicates a resource already exists
func IsAlreadyExistsError(err error) bool {
	return apierrors.IsAlreadyExists(err)
}

// IsNotFoundError checks if the error indicates a resource was not found
func IsNotFoundError(err error) bool {
	return apierrors.IsNotFound(err)
}

// IsInvalidError checks if the error indicates invalid input
func IsInvalidError(err error) bool {
	return apierrors.IsInvalid(err)
}

// MapKubernetesErrorForDelete maps Kubernetes API errors to DeleteVM responses
func MapKubernetesErrorForDelete(err error) server.DeleteVMResponseObject {
	if err == nil {
		return nil
	}

	var statusErr *apierrors.StatusError
	if !errors.As(err, &statusErr) {
		// Non-Kubernetes error, treat as internal server error
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		typ := "about:blank"
		detail := err.Error()
		return server.DeleteVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}
	}

	switch statusErr.ErrStatus.Code {
	case http.StatusNotFound:
		// 404 - Resource not found
		status := http.StatusNotFound
		title := "Not Found"
		typ := "about:blank"
		detail := statusErr.ErrStatus.Message
		return server.DeleteVM404ApplicationProblemPlusJSONResponse{
			Title:  title,
			Type:   typ,
			Status: &status,
			Detail: &detail,
		}

	default:
		// Any other Kubernetes error, treat as internal server error
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		typ := "about:blank"
		detail := "Failed to delete virtual machine"
		return server.DeleteVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}
	}
}

// MapKubernetesErrorForGet maps Kubernetes API errors to GetVM responses
func MapKubernetesErrorForGet(err error) server.GetVMResponseObject {
	if err == nil {
		return nil
	}

	var statusErr *apierrors.StatusError
	if !errors.As(err, &statusErr) {
		// Non-Kubernetes error, treat as internal server error
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		typ := "about:blank"
		detail := err.Error()
		return server.GetVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}
	}

	switch statusErr.ErrStatus.Code {
	case http.StatusNotFound:
		// 404 - Resource not found
		status := http.StatusNotFound
		title := "Not Found"
		typ := "about:blank"
		detail := statusErr.ErrStatus.Message
		return server.GetVM404ApplicationProblemPlusJSONResponse{
			Title:  title,
			Type:   typ,
			Status: &status,
			Detail: &detail,
		}

	default:
		// Any other Kubernetes error, treat as internal server error
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		typ := "about:blank"
		detail := "Failed to retrieve virtual machine"
		return server.GetVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}
	}
}

// MapKubernetesErrorForApply maps Kubernetes API errors to ApplyVM responses
func MapKubernetesErrorForApply(err error) server.ApplyVMResponseObject {
	if err == nil {
		return nil
	}

	var statusErr *apierrors.StatusError
	if !errors.As(err, &statusErr) {
		// Non-Kubernetes error, treat as internal server error
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		typ := "about:blank"
		detail := err.Error()
		return &server.ApplyVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}
	}

	switch statusErr.ErrStatus.Code {
	case http.StatusConflict:
		// 409 - Resource already exists
		status := http.StatusConflict
		title := "Conflict"
		typ := "about:blank"
		detail := statusErr.ErrStatus.Message
		return &server.ApplyVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}

	case http.StatusUnprocessableEntity:
		// 422 - Validation error
		status := http.StatusUnprocessableEntity
		title := "Validation Error"
		typ := "about:blank"
		detail := statusErr.ErrStatus.Message
		return &server.ApplyVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}

	case http.StatusBadRequest:
		// 400 - Bad request
		status := http.StatusBadRequest
		title := "Bad Request"
		typ := "about:blank"
		detail := statusErr.ErrStatus.Message
		return &server.ApplyVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}

	default:
		// Any other Kubernetes error, treat as internal server error
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		typ := "about:blank"
		detail := "Failed to apply virtual machine"
		return &server.ApplyVMdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}
	}
}

// MapKubernetesErrorForList maps Kubernetes API errors to ListVMs responses
func MapKubernetesErrorForList(err error) server.ListVMsResponseObject {
	if err == nil {
		return nil
	}

	var statusErr *apierrors.StatusError
	if !errors.As(err, &statusErr) {
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		typ := "about:blank"
		detail := err.Error()
		return &server.ListVMsdefaultApplicationProblemPlusJSONResponse{
			Body: server.Error{
				Title:  title,
				Type:   typ,
				Status: &status,
				Detail: &detail,
			},
			StatusCode: status,
		}
	}

	status := http.StatusInternalServerError
	title := "Internal Server Error"
	typ := "about:blank"
	detail := "Failed to list virtual machines"
	return &server.ListVMsdefaultApplicationProblemPlusJSONResponse{
		Body: server.Error{
			Title:  title,
			Type:   typ,
			Status: &status,
			Detail: &detail,
		},
		StatusCode: status,
	}
}