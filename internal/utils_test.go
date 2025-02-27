// Copyright Contributors to the Open Cluster Management project
package internal

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"open-cluster-management.io/ocm-kustomize-generator-plugins/internal/types"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	t.Helper()
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func assertReflectEqual(t *testing.T, a interface{}, b interface{}) {
	t.Helper()
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("%s != %s", a, b)
	}
}

func TestGetPolicyTemplate(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestFiles := []types.Manifest{}
	manifestFilesMustNotHave := []types.Manifest{}
	for i, enemy := range []string{"goldfish", "potato"} {
		manifestPath := path.Join(tmpDir, fmt.Sprintf("configmap%d.yaml", i))
		manifestYAML := fmt.Sprintf(
			`
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-configmap
data:
  game.properties: enemies=%s
`,
			enemy,
		)
		err := ioutil.WriteFile(manifestPath, []byte(manifestYAML), 0o666)
		if err != nil {
			t.Fatalf("Failed to write %s", manifestPath)
		}

		// The applyDefaults method would normally fill in ComplianceType on each manifest entry.
		manifestFiles = append(
			manifestFiles, types.Manifest{
				ComplianceType:         "musthave",
				MetadataComplianceType: "mustonlyhave",
				Path:                   manifestPath,
			},
		)
		manifestFilesMustNotHave = append(
			manifestFilesMustNotHave,
			types.Manifest{
				ComplianceType:         "mustnothave",
				MetadataComplianceType: "musthave",
				Path:                   manifestPath,
			},
		)
	}

	// Write a bogus file to ensure it is not picked up when creating the policy
	// template
	bogusFilePath := path.Join(tmpDir, "README.md")
	err := ioutil.WriteFile(bogusFilePath, []byte("# My Manifests"), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", bogusFilePath)
	}

	// Test both passing in individual files and a flat directory
	tests := []struct {
		ExpectedComplianceType         string
		ExpectedMetadataComplianceType string
		Manifests                      []types.Manifest
	}{
		{
			ExpectedComplianceType:         "musthave",
			ExpectedMetadataComplianceType: "mustonlyhave",
			Manifests:                      manifestFiles,
		},
		{
			ExpectedComplianceType:         "mustnothave",
			ExpectedMetadataComplianceType: "musthave",
			Manifests:                      manifestFilesMustNotHave,
		},
		// The applyDefaults method would normally fill in ComplianceType on each manifest entry.
		{
			ExpectedComplianceType:         "musthave",
			ExpectedMetadataComplianceType: "",
			Manifests:                      []types.Manifest{{ComplianceType: "musthave", Path: tmpDir}},
		},
	}
	// test ConsolidateManifests = true (default case)
	// policyTemplates will have only one policyTemplate
	// and two objTemplate under this policyTemplate
	for _, test := range tests {
		policyConf := types.PolicyConfig{
			ComplianceType:       "musthave",
			ConsolidateManifests: true,
			Manifests:            test.Manifests,
			Name:                 "policy-app-config",
			RemediationAction:    "inform",
			Severity:             "low",
		}

		policyTemplates, err := getPolicyTemplates(&policyConf)
		if err != nil {
			t.Fatalf("Failed to get the policy templates: %v", err)
		}
		assertEqual(t, len(policyTemplates), 1)

		policyTemplate := policyTemplates[0]
		objdef := policyTemplate["objectDefinition"]
		assertEqual(t, objdef["metadata"].(map[string]string)["name"], "policy-app-config")
		spec, ok := objdef["spec"].(map[string]interface{})
		if !ok {
			t.Fatal("The spec field is an invalid format")
		}
		assertEqual(t, spec["remediationAction"], "inform")
		assertEqual(t, spec["severity"], "low")
		objTemplates, ok := spec["object-templates"].([]map[string]interface{})
		if !ok {
			t.Fatal("The object-templates field is an invalid format")
		}
		assertEqual(t, len(objTemplates), 2)
		assertEqual(t, objTemplates[0]["complianceType"], test.ExpectedComplianceType)
		if test.ExpectedMetadataComplianceType != "" {
			assertEqual(t, objTemplates[0]["metadataComplianceType"], test.ExpectedMetadataComplianceType)
		} else {
			assertEqual(t, objTemplates[0]["metadataComplianceType"], nil)
		}
		kind1, ok := objTemplates[0]["objectDefinition"].(map[string]interface{})["kind"]
		if !ok {
			t.Fatal("The objectDefinition field is an invalid format")
		}
		assertEqual(t, kind1, "ConfigMap")
		assertEqual(t, objTemplates[1]["complianceType"], test.ExpectedComplianceType)
		if test.ExpectedMetadataComplianceType != "" {
			assertEqual(t, objTemplates[0]["metadataComplianceType"], test.ExpectedMetadataComplianceType)
		} else {
			assertEqual(t, objTemplates[0]["metadataComplianceType"], nil)
		}
		kind2, ok := objTemplates[1]["objectDefinition"].(map[string]interface{})["kind"]
		if !ok {
			t.Fatal("The objectDefinition field is an invalid format")
		}
		assertEqual(t, kind2, "ConfigMap")
	}
}

func TestGetPolicyTemplateNoConsolidate(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestFiles := []types.Manifest{}
	for i, enemy := range []string{"goldfish", "potato"} {
		manifestPath := path.Join(tmpDir, fmt.Sprintf("configmap%d.yaml", i))
		manifestYAML := fmt.Sprintf(
			`
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-configmap
data:
  game.properties: enemies=%s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-configmap2
data:
  game.properties: enemies=%s
`,
			enemy,
			enemy,
		)
		err := ioutil.WriteFile(manifestPath, []byte(manifestYAML), 0o666)
		if err != nil {
			t.Fatalf("Failed to write %s", manifestPath)
		}

		// The applyDefaults method would normally fill in ComplianceType on each manifest entry.
		manifestFiles = append(
			manifestFiles, types.Manifest{
				ComplianceType:         "musthave",
				MetadataComplianceType: "mustonlyhave",
				Path:                   manifestPath,
			},
		)
	}

	// Write a bogus file to ensure it is not picked up when creating the policy
	// template
	bogusFilePath := path.Join(tmpDir, "README.md")
	err := ioutil.WriteFile(bogusFilePath, []byte("# My Manifests"), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", bogusFilePath)
	}

	// Test both passing in individual files and a flat directory
	tests := []struct {
		Manifests []types.Manifest
	}{
		{Manifests: manifestFiles},
		// The applyDefaults method would normally fill in ComplianceType on each manifest entry.
		{
			Manifests: []types.Manifest{{
				ComplianceType:         "musthave",
				MetadataComplianceType: "mustonlyhave",
				Path:                   tmpDir,
			}},
		},
	}

	// test ConsolidateManifests = false case
	// policyTemplates will skip the consolidation and have four policyTemplate
	// and each policyTemplate has only one objTemplate
	for _, test := range tests {
		policyConf := types.PolicyConfig{
			ComplianceType:         "musthave",
			MetadataComplianceType: "musthave",
			ConsolidateManifests:   false,
			Manifests:              test.Manifests,
			Name:                   "policy-app-config",
			RemediationAction:      "inform",
			Severity:               "low",
		}

		policyTemplates, err := getPolicyTemplates(&policyConf)
		if err != nil {
			t.Fatalf("Failed to get the policy templates: %v", err)
		}
		assertEqual(t, len(policyTemplates), 4)

		for i := 0; i < len(policyTemplates); i++ {
			policyTemplate := policyTemplates[i]
			objdef := policyTemplate["objectDefinition"]
			name := "policy-app-config"
			if i > 0 {
				name += fmt.Sprintf("%d", i+1)
			}
			assertEqual(t, objdef["metadata"].(map[string]string)["name"], name)
			spec, ok := objdef["spec"].(map[string]interface{})
			if !ok {
				t.Fatal("The spec field is an invalid format")
			}
			assertEqual(t, spec["remediationAction"], "inform")
			assertEqual(t, spec["severity"], "low")
			objTemplates, ok := spec["object-templates"].([]map[string]interface{})
			if !ok {
				t.Fatal("The object-templates field is an invalid format")
			}
			assertEqual(t, len(objTemplates), 1)
			assertEqual(t, objTemplates[0]["complianceType"], "musthave")
			assertEqual(t, objTemplates[0]["metadataComplianceType"], "mustonlyhave")
			kind1, ok := objTemplates[0]["objectDefinition"].(map[string]interface{})["kind"]
			if !ok {
				t.Fatal("The objectDefinition field is an invalid format")
			}
			assertEqual(t, kind1, "ConfigMap")
		}
	}
}

func TestIsPolicyTypeManifest(t *testing.T) {
	t.Parallel()

	invalidAPI := []string{
		"policy.open-cluster-management.io/v1",
		"apps.open-cluster-management.io/v1",
	}

	invalidKind := []string{
		"CertificatePolicy",
		"IamPolicy",
	}

	tests := []struct {
		apiVersion     string
		kind           string
		invalidAPI     []string
		invalidKind    []string
		expectedFlag   bool
		expectedErrMsg string
	}{
		{"policy.open-cluster-management.io/v1", "IamPolicy", nil, nil, true, ""},
		{"policy.open-cluster-management.io/v1", "CertificatePolicy", nil, nil, true, ""},
		{"policy.open-cluster-management.io/v1", "ConfigurationPolicy", nil, nil, true, ""},
		{"policy.open-cluster-management.io/v1", "Policy", nil, nil, false, ""},
		{"apps.open-cluster-management.io/v1", "PlacementRule", nil, nil, false, ""},
		{"", "", nil, nil, false, ""},
		{"", "IamPolicy", invalidAPI, nil, false, "invalid non-string apiVersion format"},
		{"policy.open-cluster-management.io/v1", "", nil, invalidKind, false, "invalid non-string kind format"},
	}

	for _, test := range tests {
		test := test
		t.Run(
			fmt.Sprintf("apiVersion=%s, kind=%s", test.apiVersion, test.kind),
			func(t *testing.T) {
				t.Parallel()
				manifest := map[string]interface{}{}

				if test.invalidAPI == nil {
					manifest["apiVersion"] = test.apiVersion
				} else {
					manifest["apiVersion"] = test.invalidAPI
				}

				if test.invalidKind == nil {
					manifest["kind"] = test.kind
				} else {
					manifest["kind"] = test.invalidKind
				}

				isPolicyType, err := isPolicyTypeManifest(manifest)
				assertEqual(t, isPolicyType, test.expectedFlag)

				if test.expectedErrMsg == "" {
					assertEqual(t, err, nil)
				} else {
					assertEqual(t, err.Error(), test.expectedErrMsg)
				}
			},
		)
	}
}

func TestGetPolicyTemplateFromPolicyTypeManifest(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestFiles := []types.Manifest{}
	createIamPolicyManifest(t, tmpDir, "iamKindManifest.yaml")
	// Test manifest is non-root IAM policy type.
	IamManifestPath := path.Join(tmpDir, "iamKindManifest.yaml")

	manifestFiles = append(
		manifestFiles, types.Manifest{Path: IamManifestPath},
	)

	// Test both passing in individual files and a flat directory.
	tests := []struct {
		Manifests []types.Manifest
	}{
		{Manifests: manifestFiles},
		{
			Manifests: []types.Manifest{{Path: tmpDir}},
		},
	}

	for _, test := range tests {
		policyConf := types.PolicyConfig{
			Manifests:         test.Manifests,
			Name:              "policy-limitclusteradmin",
			RemediationAction: "inform",
			Severity:          "low",
		}

		policyTemplates, err := getPolicyTemplates(&policyConf)
		if err != nil {
			t.Fatalf("Failed to get the policy templates: %v", err)
		}
		assertEqual(t, len(policyTemplates), 1)

		IamPolicyTemplate := policyTemplates[0]
		IamObjdef := IamPolicyTemplate["objectDefinition"]
		assertEqual(t, IamObjdef["apiVersion"], "policy.open-cluster-management.io/v1")
		// kind will not be overridden by "ConfigurationPolicy".
		assertEqual(t, IamObjdef["kind"], "IamPolicy")
		assertEqual(t, IamObjdef["metadata"].(map[string]interface{})["name"], "policy-limitclusteradmin-example")
		IamSpec, ok := IamObjdef["spec"].(map[string]interface{})
		if !ok {
			t.Fatal("The spec field is an invalid format")
		}
		// remediationAction will not be overridden by policyConf.
		assertEqual(t, IamSpec["remediationAction"], "enforce")
		// severity will not be overridden by policyConf.
		assertEqual(t, IamSpec["severity"], "medium")
		assertEqual(t, IamSpec["maxClusterRoleBindingUsers"], 5)
		namespaceSelector, ok := IamSpec["namespaceSelector"].(map[string]interface{})
		if !ok {
			t.Fatal("The namespaceSelector field is an invalid format")
		}
		assertReflectEqual(t, namespaceSelector["include"], []interface{}{"*"})
		assertReflectEqual(t, namespaceSelector["exclude"], []interface{}{"kube-*", "openshift-*"})
	}
}

func TestGetPolicyTemplatePatches(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestPath := path.Join(tmpDir, "configmap.yaml")
	manifestYAML := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-configmap
data:
  game.properties: enemies=potato
`
	err := ioutil.WriteFile(manifestPath, []byte(manifestYAML), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", manifestPath)
	}

	patches := []map[string]interface{}{
		{
			"metadata": map[string]interface{}{
				"labels": map[string]string{"chandler": "bing"},
			},
		},
		{
			"metadata": map[string]interface{}{
				"annotations": map[string]string{"monica": "geller"},
			},
		},
	}
	manifests := []types.Manifest{
		{Path: manifestPath, Patches: patches},
	}
	policyConf := types.PolicyConfig{
		Manifests: manifests,
		Name:      "policy-app-config",
	}

	policyTemplates, err := getPolicyTemplates(&policyConf)
	if err != nil {
		t.Fatalf("Failed to get the policy templates: %v", err)
	}
	assertEqual(t, len(policyTemplates), 1)

	policyTemplate := policyTemplates[0]
	objdef := policyTemplate["objectDefinition"]
	assertEqual(t, objdef["metadata"].(map[string]string)["name"], "policy-app-config")
	spec, ok := objdef["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("The spec field is an invalid format")
	}

	objTemplates, ok := spec["object-templates"].([]map[string]interface{})
	if !ok {
		t.Fatal("The object-templates field is an invalid format")
	}
	assertEqual(t, len(objTemplates), 1)

	objDef, ok := objTemplates[0]["objectDefinition"].(map[string]interface{})
	if !ok {
		t.Fatal("The objectDefinition field is an invalid format")
	}

	metadata, ok := objDef["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("The metadata field is an invalid format")
	}

	labels, ok := metadata["labels"].(map[string]interface{})
	if !ok {
		t.Fatal("The labels field is an invalid format")
	}
	assertReflectEqual(t, labels, map[string]interface{}{"chandler": "bing"})

	annotations, ok := metadata["annotations"].(map[string]interface{})
	if !ok {
		t.Fatal("The annotations field is an invalid format")
	}
	assertReflectEqual(t, annotations, map[string]interface{}{"monica": "geller"})
}

func TestGetPolicyTemplateMetadataPatches(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestPath := path.Join(tmpDir, "patch-configmap.yaml")
	manifestYAML := `
---
apiVersion: v1
kind: configmap
metadata:
  name: test-configmap
  namespace: test-namespace
data:
  image: "quay.io/potatos1"
`
	err := ioutil.WriteFile(manifestPath, []byte(manifestYAML), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", manifestPath)
	}

	patches := []map[string]interface{}{
		{
			"metadata": map[string]interface{}{
				"name":      "patch-configmap",
				"namespace": "patch-namespace",
			},
			"data": map[string]interface{}{
				"image": "quay.io/potatos2",
			},
		},
	}

	manifests := []types.Manifest{
		{Path: manifestPath, Patches: patches},
	}
	policyConf := types.PolicyConfig{
		Manifests: manifests,
		Name:      "policy-app-config",
	}

	policyTemplates, err := getPolicyTemplates(&policyConf)
	if err != nil {
		t.Fatalf("Failed to get the policy templates: %v ", err)
	}
	assertEqual(t, len(policyTemplates), 1)

	policyTemplate := policyTemplates[0]
	objdef := policyTemplate["objectDefinition"]
	assertEqual(t, objdef["metadata"].(map[string]string)["name"], "policy-app-config")
	spec, ok := objdef["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("The spec field is an invalid format")
	}

	objTemplates, ok := spec["object-templates"].([]map[string]interface{})
	if !ok {
		t.Fatal("The object-templates field is an invalid format")
	}
	assertEqual(t, len(objTemplates), 1)

	objDef, ok := objTemplates[0]["objectDefinition"].(map[string]interface{})
	if !ok {
		t.Fatal("The objectDefinition field is an invalid format")
	}

	metadata, ok := objDef["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("The metadata field is an invalid format")
	}

	name, ok := metadata["name"].(string)
	if !ok {
		t.Fatal("The metadata.name field is an invalid format")
	}
	assertEqual(t, name, "patch-configmap")

	namespace, ok := metadata["namespace"].(string)
	if !ok {
		t.Fatal("The metadata.namespace field is an invalid format")
	}
	assertEqual(t, namespace, "patch-namespace")

	data, ok := objDef["data"].(map[string]interface{})
	if !ok {
		t.Fatal("The data field is an invalid format")
	}

	image, ok := data["image"].(string)
	if !ok {
		t.Fatal("The data.image field is an invalid format")
	}
	assertEqual(t, image, "quay.io/potatos2")
}

func TestGetPolicyTemplateMetadataPatchesFail(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestPath := path.Join(tmpDir, "multi-configmaps.yaml")
	manifestYAML := `
---
apiVersion: v1
kind: configmap
metadata:
  name: test-configmap
  namespace: test-namespace
data:
  image: "quay.io/potatos1"
---
apiVersion: v1
kind: configmap
metadata:
  name: test2-configmap
  namespace: test2-namespace
data:
  image: "quay.io/potatos1"
`
	err := ioutil.WriteFile(manifestPath, []byte(manifestYAML), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", manifestPath)
	}

	patches := []map[string]interface{}{
		{
			"metadata": map[string]interface{}{
				"name":      "patch-configmap",
				"namespace": "patch-namespace",
			},
			"data": map[string]interface{}{
				"image": "quay.io/potatos2",
			},
		},
	}

	manifests := []types.Manifest{
		{Path: manifestPath, Patches: patches},
	}
	policyConf := types.PolicyConfig{
		Manifests: manifests,
		Name:      "policy-app-config",
	}

	_, err = getPolicyTemplates(&policyConf)
	assertEqual(t, err != nil, true)
}

func TestGetPolicyTemplateKyverno(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestPath := path.Join(tmpDir, "kyverno.yaml")
	manifestYAML := `
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: my-awesome-policy`

	err := ioutil.WriteFile(manifestPath, []byte(manifestYAML), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", manifestPath)
	}

	policyConf := types.PolicyConfig{
		ComplianceType:        "musthave",
		InformKyvernoPolicies: true,
		Manifests:             []types.Manifest{{Path: manifestPath}},
		Name:                  "policy-kyverno-config",
		RemediationAction:     "enforce",
		Severity:              "low",
	}

	policyTemplates, err := getPolicyTemplates(&policyConf)
	if err != nil {
		t.Fatalf("Failed to get the policy templates: %v", err)
	}
	assertEqual(t, len(policyTemplates), 2)

	// This is not an in-depth test since the Kyverno expansion is tested elsewhere. This is to
	// to test that glue code is working as expected.
	expandedPolicyTemplate := policyTemplates[1]
	objdef := expandedPolicyTemplate["objectDefinition"]
	spec, ok := objdef["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("The spec field is an invalid format")
	}
	objTemplates, ok := spec["object-templates"].([]map[string]interface{})
	if !ok {
		t.Fatal("The object-templates field is an invalid format")
	}
	assertEqual(t, len(objTemplates), 2)
	assertEqual(t, objTemplates[0]["complianceType"], "mustnothave")
	assertEqual(t, objTemplates[0]["metadataComplianceType"], nil)
	kind1, ok := objTemplates[0]["objectDefinition"].(map[string]interface{})["kind"]
	if !ok {
		t.Fatal("The objectDefinition field is an invalid format")
	}
	assertEqual(t, kind1, "ClusterPolicyReport")

	assertEqual(t, objTemplates[1]["complianceType"], "mustnothave")
	assertEqual(t, objTemplates[1]["metadataComplianceType"], nil)
	kind2, ok := objTemplates[1]["objectDefinition"].(map[string]interface{})["kind"]
	if !ok {
		t.Fatal("The objectDefinition field is an invalid format")
	}
	assertEqual(t, kind2, "PolicyReport")
}

func TestGetPolicyTemplateNoManifests(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	policyConf := types.PolicyConfig{
		ComplianceType:    "musthave",
		Manifests:         []types.Manifest{{Path: tmpDir}},
		Name:              "policy-app-config",
		RemediationAction: "inform",
		Severity:          "low",
	}

	_, err := getPolicyTemplates(&policyConf)
	if err == nil {
		t.Fatal("Expected an error but did not get one")
	}

	expected := "the policy policy-app-config must specify at least one non-empty manifest file"
	assertEqual(t, err.Error(), expected)
}

func TestGetPolicyTemplateInvalidPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestPath := path.Join(tmpDir, "does-not-exist.yaml")
	policyConf := types.PolicyConfig{
		ComplianceType:    "musthave",
		Manifests:         []types.Manifest{{ComplianceType: "musthave", Path: manifestPath}},
		Name:              "policy-app-config",
		RemediationAction: "inform",
		Severity:          "low",
	}

	_, err := getPolicyTemplates(&policyConf)
	if err == nil {
		t.Fatal("Expected an error but did not get one")
	}

	expected := fmt.Sprintf("failed to read the manifest path %s", manifestPath)
	assertEqual(t, err.Error(), expected)
}

func TestGetPolicyTemplateInvalidManifest(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestPath := path.Join(tmpDir, "configmap.yaml")
	// Ensure an error is returned when there is an invalid manifest file
	err := ioutil.WriteFile(manifestPath, []byte("$i am not YAML!"), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", manifestPath)
	}

	policyConf := types.PolicyConfig{
		ComplianceType:    "musthave",
		Manifests:         []types.Manifest{{Path: manifestPath}},
		Name:              "policy-app-config",
		RemediationAction: "inform",
		Severity:          "low",
	}

	_, err = getPolicyTemplates(&policyConf)
	if err == nil {
		t.Fatal("Expected an error but did not get one")
	}

	expected := fmt.Sprintf(
		"failed to decode the manifest file at %s: the input manifests must be in the format of "+
			"YAML objects", manifestPath,
	)
	assertEqual(t, err.Error(), expected)
}

func TestUnmarshalManifestFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestsPath := path.Join(tmpDir, "configmaps.yaml")
	yamlContent := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-configmap
data:
  game.properties: |
    enemies=goldfish
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-configmap2
data:
  game.properties: |
    enemies=potato
`
	err := ioutil.WriteFile(manifestsPath, []byte(yamlContent), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", manifestsPath)
	}

	manifests, err := unmarshalManifestFile(manifestsPath)
	if err != nil {
		t.Fatalf("Failed to unmarshal the YAML content, got: %v", err)
	}

	assertEqual(t, len(*manifests), 2)
	name1, _, _ := unstructured.NestedString((*manifests)[0], "metadata", "name")
	assertEqual(t, name1, "my-configmap")
	name2, _, _ := unstructured.NestedString((*manifests)[1], "metadata", "name")
	assertEqual(t, name2, "my-configmap2")
}

func TestUnmarshalManifestFileUnreadable(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestsPath := path.Join(tmpDir, "configmaps.yaml")
	_, err := unmarshalManifestFile(manifestsPath)
	if err == nil {
		t.Fatal("Expected an error but did not get one")
	}

	expected := fmt.Sprintf("failed to read the manifest file %s", manifestsPath)
	assertEqual(t, err.Error(), expected)
}

func TestUnmarshalManifestFileInvalidYAML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestPath := path.Join(tmpDir, "configmaps.yaml")
	yamlContent := `$I am not YAML`
	err := ioutil.WriteFile(manifestPath, []byte(yamlContent), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", manifestPath)
	}

	_, err = unmarshalManifestFile(manifestPath)
	if err == nil {
		t.Fatal("Expected an error but did not get one")
	}
}

func TestUnmarshalManifestFileNotObject(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	manifestPath := path.Join(tmpDir, "configmaps.yaml")
	yamlContent := `- i am an array`
	err := ioutil.WriteFile(manifestPath, []byte(yamlContent), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", manifestPath)
	}

	_, err = unmarshalManifestFile(manifestPath)
	if err == nil {
		t.Fatal("Expected an error but did not get one")
	}

	expected := fmt.Sprintf(
		"failed to decode the manifest file at %s: the input manifests must be in the format of "+
			"YAML objects", manifestPath,
	)
	assertEqual(t, err.Error(), expected)
}

// nolint: paralleltest
func TestVerifyManifestPath(t *testing.T) {
	baseDirectory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to evaluate symlinks for the base directory: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		err := os.Chdir(cwd)
		if err != nil {
			// panic since this could affect other tests that haven't yet run
			panic(fmt.Sprintf("Couldn't go back to the original working directory: %v", err))
		}
	}()

	// Set up directory structure, with 'workingdir' as target directory:
	// baseDirectory (t.TempDir())
	// ├── workingdir
	// │   └── subdir
	// └── otherdir
	workingDir := path.Join(baseDirectory, "workingdir")
	subDir := path.Join(workingDir, "subdir")
	otherDir := path.Join(baseDirectory, "otherdir")
	err = os.MkdirAll(subDir, 0777)
	if err != nil {
		t.Fatalf("Failed to create the directory structure %s: %v", subDir, err)
	}
	err = os.Mkdir(otherDir, 0777)
	if err != nil {
		t.Fatalf("Failed to create the directory structure %s: %v", otherDir, err)
	}

	// Create files in baseDirectory/workingdir and baseDirectory/otherdir
	manifestPath := path.Join(workingDir, "configmap.yaml")
	yamlContent := "---\nkind: ConfigMap"
	err = ioutil.WriteFile(manifestPath, []byte(yamlContent), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", manifestPath)
	}
	otherManifestPath := path.Join(otherDir, "configmap.yaml")
	err = ioutil.WriteFile(otherManifestPath, []byte(yamlContent), 0o666)
	if err != nil {
		t.Fatalf("Failed to write %s", otherManifestPath)
	}

	err = os.Chdir(workingDir)
	if err != nil {
		t.Fatalf("Failed to change the working directory to %s: %v", workingDir, err)
	}

	grandParentDir := path.Join("..", "..")
	relOtherManifestPath := path.Join("..", "otherdir", "configmap.yaml")

	tests := []struct {
		ManifestPath   string
		ExpectedErrMsg string
	}{
		{manifestPath, ""},
		{"configmap.yaml", ""},
		{"subdir", ""},
		{
			"..",
			"the manifest path .. is not in the same directory tree as the kustomization.yaml file",
		},
		{
			grandParentDir,
			fmt.Sprintf("the manifest path %s is not in the same directory tree as the kustomization.yaml file", grandParentDir),
		},
		{
			baseDirectory,
			fmt.Sprintf("the manifest path %s is not in the same directory tree as the kustomization.yaml file", baseDirectory),
		},
		{
			otherManifestPath,
			fmt.Sprintf("the manifest path %s is not in the same directory tree as the kustomization.yaml file", otherManifestPath),
		},
		{
			relOtherManifestPath,
			fmt.Sprintf("the manifest path %s is not in the same directory tree as the kustomization.yaml file", relOtherManifestPath),
		},
		{
			otherDir,
			fmt.Sprintf("the manifest path %s is not in the same directory tree as the kustomization.yaml file", otherDir),
		},
	}

	for _, test := range tests {
		test := test
		// nolint: paralleltest
		t.Run(
			"manifestPath="+test.ManifestPath,
			func(t *testing.T) {
				err := verifyManifestPath(workingDir, test.ManifestPath)
				if err == nil {
					assertEqual(t, "", test.ExpectedErrMsg)
				} else {
					assertEqual(t, err.Error(), test.ExpectedErrMsg)
				}
			},
		)
	}
}
