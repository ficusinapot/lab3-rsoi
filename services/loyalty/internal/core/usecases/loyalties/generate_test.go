package loyalties

//go:generate go tool mockgen -source=../../../models/dbifc/loyalties.go -destination=repository_mock_test.go -package=loyalties -mock_names=Loyalties=MockRepository
