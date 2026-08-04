package handlers

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOpenAPIDocumentsQualityRecommendationContract(t *testing.T) {
	path, ok := openAPIPaths()["/api/assets/quality-recommendation"].(gin.H)
	if !ok {
		t.Fatal("quality recommendation path is missing from OpenAPI")
	}
	post, ok := path["post"].(gin.H)
	if !ok || post["operationId"] != "recommendEncoderQuality" {
		t.Fatalf("quality recommendation operation is incomplete: %#v", path)
	}
	components := openAPIComponents()
	schemas, ok := components["schemas"].(gin.H)
	if !ok {
		t.Fatal("OpenAPI schemas are missing")
	}
	for _, name := range []string{"QualityRecommendationInput", "EncoderRecommendation", "QualityRecommendationResponse"} {
		if _, exists := schemas[name]; !exists {
			t.Fatalf("OpenAPI schema %s is missing", name)
		}
	}
}
