/*
Copyright 2026 The AlaudaDevops Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package namespace

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// TestLabelNamespaceMembershipResolverMatchesConfiguredLabel verifies label-based membership resolution.
func TestLabelNamespaceMembershipResolverMatchesConfiguredLabel(t *testing.T) {
	resolver := NewLabelNamespaceMembershipResolver(&fakeNamespaceLister{
		namespaces: []*corev1.Namespace{
			namespaceFixture("team-b", "project-a"),
			namespaceFixture("team-a", "project-a"),
			namespaceFixture("team-c", "project-b"),
		},
	}, "cpaas.io/project")

	membership, err := resolver.Resolve(context.Background(), "team-a")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if membership.Identity != "project-a" {
		t.Fatalf("membership identity = %q, want project-a", membership.Identity)
	}
	if want := []string{"team-a", "team-b"}; !reflect.DeepEqual(membership.Namespaces, want) {
		t.Fatalf("membership namespaces = %#v, want %#v", membership.Namespaces, want)
	}
}

// TestLabelNamespaceMembershipResolverFiltersInactiveAndDuplicateNamespaces verifies stable active results.
func TestLabelNamespaceMembershipResolverFiltersInactiveAndDuplicateNamespaces(t *testing.T) {
	deleting := namespaceFixture("team-deleting", "project-a")
	deleting.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	resolver := NewLabelNamespaceMembershipResolver(&fakeNamespaceLister{
		namespaces: []*corev1.Namespace{
			namespaceFixture("team-b", "project-a"),
			namespaceFixture("team-b", "project-a"),
			deleting,
			&corev1.Namespace{},
			nil,
		},
	}, "cpaas.io/project")

	membership, err := resolver.Resolve(context.Background(), "team-b")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if want := []string{"team-b"}; !reflect.DeepEqual(membership.Namespaces, want) {
		t.Fatalf("membership namespaces = %#v, want %#v", membership.Namespaces, want)
	}
}

// TestLabelNamespaceMembershipResolverRejectsMissingOwnerMembership verifies missing label diagnostics.
func TestLabelNamespaceMembershipResolverRejectsMissingOwnerMembership(t *testing.T) {
	resolver := NewLabelNamespaceMembershipResolver(&fakeNamespaceLister{
		namespaces: []*corev1.Namespace{{ObjectMeta: metav1.ObjectMeta{Name: "team-a"}}},
	}, "cpaas.io/project")

	_, err := resolver.Resolve(context.Background(), "team-a")
	if !errors.Is(err, ErrNamespaceMembershipNotFound) {
		t.Fatalf("Resolve error = %v, want ErrNamespaceMembershipNotFound", err)
	}
}

// TestLabelNamespaceMembershipResolverUsesCustomLabelKey verifies the label key is caller-configurable.
func TestLabelNamespaceMembershipResolverUsesCustomLabelKey(t *testing.T) {
	resolver := NewLabelNamespaceMembershipResolver(&fakeNamespaceLister{
		namespaces: []*corev1.Namespace{
			customNamespaceFixture("team-a", "example.io/tenant", "tenant-a"),
			customNamespaceFixture("team-b", "example.io/tenant", "tenant-a"),
		},
	}, "example.io/tenant")

	membership, err := resolver.Resolve(context.Background(), "team-a")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if membership.Identity != "tenant-a" {
		t.Fatalf("membership identity = %q, want tenant-a", membership.Identity)
	}
}

// TestLabelNamespaceMembershipResolverHonorsContext verifies cancellation is propagated.
func TestLabelNamespaceMembershipResolverHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := NewLabelNamespaceMembershipResolver(&fakeNamespaceLister{}, "cpaas.io/project")

	_, err := resolver.Resolve(ctx, "team-a")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve error = %v, want context.Canceled", err)
	}
}

// fakeNamespaceLister provides deterministic Namespace data for resolver tests.
type fakeNamespaceLister struct {
	namespaces []*corev1.Namespace
	err        error
}

// Get returns the requested Namespace from the fake store.
func (f *fakeNamespaceLister) Get(name string) (*corev1.Namespace, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, item := range f.namespaces {
		if item != nil && item.Name == name {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(corev1.Resource("namespaces"), name)
}

// List returns fake Namespaces matching selector.
func (f *fakeNamespaceLister) List(selector labels.Selector) ([]*corev1.Namespace, error) {
	if f.err != nil {
		return nil, f.err
	}
	var result []*corev1.Namespace
	for _, item := range f.namespaces {
		if item != nil && selector.Matches(labels.Set(item.Labels)) {
			result = append(result, item)
		}
	}
	return result, nil
}

// namespaceFixture creates a Namespace carrying the default project label.
func namespaceFixture(name string, project string) *corev1.Namespace {
	return customNamespaceFixture(name, "cpaas.io/project", project)
}

// customNamespaceFixture creates a Namespace carrying a caller-selected label.
func customNamespaceFixture(name string, labelKey string, identity string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   name,
		Labels: map[string]string{labelKey: identity},
	}}
}
