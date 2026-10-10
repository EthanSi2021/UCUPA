// Operation names preserve the existing public and persisted vocabulary.
package binding

import (
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

func KindName(kind elementfact.OperationKind) string {
	switch kind {
	case elementfact.OperationCreateRoot:
		return "create-root"
	case elementfact.OperationCreateAnchor:
		return "create-anchor"
	case elementfact.OperationCreateEntity:
		return "create-entity"
	case elementfact.OperationUpdateAnchor:
		return "update-anchor"
	case elementfact.OperationUpdateEntity:
		return "update-entity"
	case elementfact.OperationDeleteAnchor:
		return "delete-anchor"
	case elementfact.OperationDeleteEntity:
		return "delete-entity"
	default:
		return ""
	}
}

func KindFromName(name string) elementfact.OperationKind {
	switch name {
	case "create-root":
		return elementfact.OperationCreateRoot
	case "create-anchor":
		return elementfact.OperationCreateAnchor
	case "create-entity":
		return elementfact.OperationCreateEntity
	case "update-anchor":
		return elementfact.OperationUpdateAnchor
	case "update-entity":
		return elementfact.OperationUpdateEntity
	case "delete-anchor":
		return elementfact.OperationDeleteAnchor
	case "delete-entity":
		return elementfact.OperationDeleteEntity
	default:
		return elementfact.OperationInvalid
	}
}
