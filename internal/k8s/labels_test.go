package k8s

import (
	"strings"
	"testing"
)

func TestInstanceLabels(t *testing.T) {
	labels := InstanceLabels("myapp")

	expected := map[string]string{
		LabelManagedBy: ManagedByValue,
		LabelInstance:  "myapp",
		LabelName:      NameValue,
		LabelComponent: ComponentValue,
	}

	if len(labels) != len(expected) {
		t.Fatalf("InstanceLabels returned %d labels, expected %d", len(labels), len(expected))
	}

	for k, v := range expected {
		if got, ok := labels[k]; !ok {
			t.Errorf("InstanceLabels missing label %q", k)
		} else if got != v {
			t.Errorf("InstanceLabels[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestInstanceSelector(t *testing.T) {
	sel := InstanceSelector("myapp")

	if !strings.Contains(sel, "app.kubernetes.io/managed-by=eclaw") {
		t.Errorf("InstanceSelector %q does not contain managed-by=eclaw", sel)
	}
	if !strings.Contains(sel, "app.kubernetes.io/instance=myapp") {
		t.Errorf("InstanceSelector %q does not contain instance=myapp", sel)
	}
}

func TestManagedSelector(t *testing.T) {
	sel := ManagedSelector()

	want := "app.kubernetes.io/managed-by=eclaw"
	if sel != want {
		t.Errorf("ManagedSelector = %q, want %q", sel, want)
	}
}
