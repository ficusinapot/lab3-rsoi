package payments

//go:generate go tool mockgen -source=../../../models/dbifc/payments.go -destination=repository_mock_test.go -package=payments -mock_names=Payments=MockRepository
