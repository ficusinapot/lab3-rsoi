package loyalties

//go:generate go tool mockgen -source=../../models/coreifc/loyalties.go -destination=service_mock_test.go -package=loyalties -mock_names=Loyalties=MockService
