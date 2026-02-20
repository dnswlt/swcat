package lint

import catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"

var (
	KnownCustomChecks = map[string]CustomCheckFunc{
		"alwaysFail": CheckAlwaysFail,
	}
)

func CheckAlwaysFail(e *catalog_pb.Entity) []Finding {
	return []Finding{
		{
			RuleName: "AlwaysFail",
			Severity: SeverityInfo,
			Message:  "This check always fails. Use it only for testing your linting config.",
		},
	}
}
