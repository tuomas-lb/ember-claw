package server

import "github.com/google/uuid"

// assignSessionKey returns clientKey if non-empty, otherwise generates a new
// session key with the given prefix: "<prefix>:<uuid>".
// Used by Chat and Query handlers to enforce per-stream session isolation (GRPC-05).
func assignSessionKey(clientKey, prefix string) string {
	if clientKey != "" {
		return clientKey
	}
	return prefix + ":" + uuid.New().String()
}
