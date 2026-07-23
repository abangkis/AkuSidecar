package mediaprovenance

import "testing"

func TestParseC2PAToolOutputClassifiesGeneratedImage(t *testing.T) {
	result, err := ParseC2PAToolOutput([]byte(`{
	  "active_manifest":"urn:uuid:test",
	  "manifests":{"urn:uuid:test":{"assertions":[
	    {"label":"c2pa.actions","data":{"actions":[{"action":"c2pa.created","digitalSourceType":"http://cv.iptc.org/newscodes/digitalsourcetype/trainedAlgorithmicMedia"}]}}
	  ]}},
	  "validation_status":[{"code":"signingCredential.trusted"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.ManifestState != "valid" || result.AIOrigin != "generated" || result.TrustState != "trusted" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestParseC2PAToolOutputClassifiesEditedImage(t *testing.T) {
	result, err := ParseC2PAToolOutput([]byte(`{
	  "active_manifest":"urn:uuid:test",
	  "manifests":{"urn:uuid:test":{"assertions":[
	    {"digitalSourceType":"http://cv.iptc.org/newscodes/digitalsourcetype/compositeWithTrainedAlgorithmicMedia"}
	  ]}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.AIOrigin != "edited" || result.TrustState != "not_evaluated" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestParseC2PAToolOutputTreatsMissingManifestAsNeutral(t *testing.T) {
	result, err := ParseC2PAToolOutput([]byte(`{"manifests":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.ManifestState != "no_manifest" || result.AIOrigin != "none" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestParseC2PAToolOutputTreatsToolNoClaimAsNeutral(t *testing.T) {
	result, err := ParseC2PAToolOutput([]byte("Error: No claim found"))
	if err != nil {
		t.Fatal(err)
	}
	if result.ManifestState != "no_manifest" || result.AIOrigin != "none" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestParseC2PAToolOutputDoesNotTrustBrokenManifest(t *testing.T) {
	result, err := ParseC2PAToolOutput([]byte(`{
	  "active_manifest":"urn:uuid:test",
	  "digitalSourceType":"http://cv.iptc.org/newscodes/digitalsourcetype/trainedAlgorithmicMedia",
	  "validation_status":[{"code":"claimSignature.mismatch"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.ManifestState != "invalid" || result.AIOrigin != "unknown" || len(result.EvidenceCodes) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestImageExtensionUsesURLThenContentType(t *testing.T) {
	if got := imageExtension("/media/photo.PNG", "application/octet-stream"); got != ".png" {
		t.Fatalf("URL extension=%q", got)
	}
	if got := imageExtension("/media/render", "image/webp; charset=binary"); got != ".webp" {
		t.Fatalf("content-type extension=%q", got)
	}
}
