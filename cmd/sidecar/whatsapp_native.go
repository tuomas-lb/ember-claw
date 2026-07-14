//go:build whatsapp_native

package main

// Blank-import the native WhatsApp channel so its factory registers with the
// PicoClaw channel manager. PicoClaw does not import this package itself (it
// would force the whatsmeow dependency on every build), so the consumer must —
// under the same `whatsapp_native` build tag the Dockerfile passes. This also
// makes a tagged build fail loudly if the whatsmeow deps are missing from
// go.mod, rather than silently producing a WhatsApp-less binary.
import _ "github.com/sipeed/picoclaw/pkg/channels/whatsapp_native"
