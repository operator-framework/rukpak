package util

import rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"

// BundlesByCreationTimestamp sorts a list of ReplicaSet by creation timestamp, using their names as a tie breaker.
type BundlesByCreationTimestamp []*rukpakv1alpha1.Bundle

func (o BundlesByCreationTimestamp) Len() int      { return len(o) }
func (o BundlesByCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o BundlesByCreationTimestamp) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
}
