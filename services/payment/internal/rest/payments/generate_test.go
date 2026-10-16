package payments

//go:generate go tool mockgen -source=../../models/coreifc/payments.go -destination=service_mock_test.go -package=payments -mock_names=Payments=MockService
