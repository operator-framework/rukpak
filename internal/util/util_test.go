package util

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

var sampleSpec = rukpakv1alpha1.BundleSpec{
	ProvisionerClassName: "sample",
	Source: rukpakv1alpha1.BundleSource{
		Type: rukpakv1alpha1.SourceTypeImage,
		Image: &rukpakv1alpha1.ImageSource{
			Ref: "non-existent",
		},
	},
}

func TestCheckDesiredBundleTemplate(t *testing.T) {
	type args struct {
		existingBundle *rukpakv1alpha1.Bundle
		desiredBundle  rukpakv1alpha1.BundleTemplate
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "True/BundleMatchesTemplate",
			args: args{
				existingBundle: &rukpakv1alpha1.Bundle{
					ObjectMeta: metav1.ObjectMeta{
						Name: "stub-6f74b48d4",
						Labels: map[string]string{
							"stub": "stub",
						},
					},
					Spec: sampleSpec,
				},
				desiredBundle: rukpakv1alpha1.BundleTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name: "stub",
						Labels: map[string]string{
							"stub": "stub",
						},
					},
					Spec: sampleSpec,
				},
			},
			want: true,
		},
		{
			name: "False/SpecDiffers",
			args: args{
				existingBundle: &rukpakv1alpha1.Bundle{
					ObjectMeta: metav1.ObjectMeta{
						Name: "stub-6dd88668d7",
						Labels: map[string]string{
							"stub": "stub",
						},
					},
					Spec: rukpakv1alpha1.BundleSpec{
						ProvisionerClassName: "non-existent-provisioner-class-name",
					},
				},
				desiredBundle: rukpakv1alpha1.BundleTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name: "stub",
						Labels: map[string]string{
							"stub": "stub",
						},
					},
					Spec: sampleSpec,
				},
			},
			want: false,
		},
		{
			name: "False/LabelsDiffer",
			args: args{
				existingBundle: &rukpakv1alpha1.Bundle{
					ObjectMeta: metav1.ObjectMeta{
						Name: "stub-6f74b48d4",
						Labels: map[string]string{
							"stub": "stub",
						},
					},
					Spec: sampleSpec,
				},
				desiredBundle: rukpakv1alpha1.BundleTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name: "stub",
						Labels: map[string]string{
							"stub": "different-value",
						},
					},
					Spec: sampleSpec,
				},
			},
			want: false,
		},
		{
			name: "False/AnnotationsDiffer",
			args: args{
				existingBundle: &rukpakv1alpha1.Bundle{
					ObjectMeta: metav1.ObjectMeta{
						Name: "stub-77c4548c75",
						Labels: map[string]string{
							"stub": "stub",
						},
						Annotations: map[string]string{
							"stub": "stub",
						},
					},
					Spec: sampleSpec,
				},
				desiredBundle: rukpakv1alpha1.BundleTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name: "stub",
						Labels: map[string]string{
							"stub": "stub",
						},
						Annotations: map[string]string{
							"stub": "",
						},
					},
					Spec: sampleSpec,
				},
			},
			want: false,
		},
		{
			name: "True/BundleMatchesTemplateHyphens",
			args: args{
				existingBundle: &rukpakv1alpha1.Bundle{
					ObjectMeta: metav1.ObjectMeta{
						Name: "stub-123-6cc4cf6797",
						Labels: map[string]string{
							"stub-123": "stub-123",
						},
					},
					Spec: sampleSpec,
				},
				desiredBundle: rukpakv1alpha1.BundleTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name: "stub-123",
						Labels: map[string]string{
							"stub-123": "stub-123",
						},
					},
					Spec: sampleSpec,
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
		// 	injectCoreLabels(tt.args.existingBundle)
		// 	// Dynamically inject the bundle template hash at runtime into the tests.
		// 	// This is due to the nature of the objects being passed in (pointers to BundleTemplates) being represented
		// 	// differently on different platforms, so hardcoding the hash values produces inconsistent results.
		// 	injectTemplateHashLabel(t, tt.args.existingBundle, tt.args.desiredBundle, tt.want)
		// 	got, err := CheckDesiredBundleTemplate(tt.args.existingBundle, tt.args.desiredBundle)
		// 	if err != nil {
		// 		t.Fatal(err)
		// 	}
			// if got != tt.want {
			// 	t.Errorf("CheckDesiredBundleTemplate() = %v, want %v", got, tt.want)
			// }
		})
	}
}

func injectCoreLabels(bundle *rukpakv1alpha1.Bundle) {
	labels := bundle.GetLabels()
	if len(labels) == 0 {
		labels = make(map[string]string)
	}
	labels[CoreOwnerKindKey] = ""
	labels[CoreOwnerNameKey] = ""
}

func injectTemplateHashLabel(t *testing.T, bundle *rukpakv1alpha1.Bundle, template rukpakv1alpha1.BundleTemplate, want bool) {
	labels := bundle.GetLabels()
	if want {
		hash, err := DeepHashObject(template)
		if err != nil {
			t.Fatal(err)
		}
		labels[CoreBundleTemplateHashKey] = hash
	} else {
		labels[CoreBundleTemplateHashKey] = "00000000"
	}
}
