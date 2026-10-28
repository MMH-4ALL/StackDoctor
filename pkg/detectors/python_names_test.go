package detectors

import (
	"context"
	"testing"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
)

func TestPythonDistributionNamesUseNormalizedSeparators(t *testing.T) {
	for _, name := range []string{"My__Package", "my..package", "my-._-package"} {
		t.Run(name, func(t *testing.T) {
			c, _ := fixtures(t, map[string]string{
				"requirements.txt": name + "==1.0.0\n",
				".venv/Lib/site-packages/my_package-1.0.0.dist-info/METADATA": "Name: my-package\nVersion: 1.0.0\n",
			})
			findings := (Python{}).Detect(context.Background(), c)
			if has(findings, "python.dependencies.missing", diagnostic.Error) || !has(findings, "python.dependencies.present", diagnostic.Pass) {
				t.Fatalf("installed distribution was not recognized: %+v", findings)
			}
		})
	}
}
