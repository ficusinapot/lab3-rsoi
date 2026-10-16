package opstream

type keyspace struct {
	operationPrefix    string
	activeSet          string
	compensationSet    string
	normalSet          string
	compensationDue    string
	cancellationDue    string
	loyaltyDue         string
	generation         string
	compensationStream string
	cancellationStream string
	loyaltyStream      string
}

func newKeyspace(prefix string) keyspace {
	return keyspace{
		operationPrefix: prefix + "operation:", activeSet: prefix + "active",
		compensationSet: prefix + "class:compensation", normalSet: prefix + "class:normal",
		compensationDue: prefix + "due:compensation", cancellationDue: prefix + "due:cancellation",
		loyaltyDue: prefix + "due:loyalty", generation: prefix + "generation",
		compensationStream: prefix + "stream:compensation", cancellationStream: prefix + "stream:cancellation",
		loyaltyStream: prefix + "stream:loyalty",
	}
}

func (k keyspace) streams() [3]string {
	return [3]string{k.compensationStream, k.cancellationStream, k.loyaltyStream}
}

func (k keyspace) operationSets() [3]string {
	return [3]string{k.activeSet, k.compensationSet, k.normalSet}
}

func (k keyspace) dueQueues() [3]string {
	return [3]string{k.compensationDue, k.cancellationDue, k.loyaltyDue}
}
