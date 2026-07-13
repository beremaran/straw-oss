package receipt

// legalTransitions is the single lifecycle policy. Persistence and object
// composition code may request a transition but must not invent one.
var legalTransitions = map[string]map[string]struct{}{
	StateUploading: {
		StateVerifying: {}, StateCancelled: {}, StateExpired: {},
	},
	StateVerifying: {
		StateVerified: {}, StateRejected: {}, StateCancelled: {}, StateExpired: {},
	},
	StateVerified: {
		StateAssigned: {}, StateCancelled: {}, StateExpired: {},
	},
	StateAssigned: {
		StateVerified: {}, StateConsumed: {}, StateExpired: {},
	},
}

func canTransition(from, to string) bool {
	_, ok := legalTransitions[from][to]

	return ok
}

func transition(record *Record, to string) error {
	if record.State == to {
		return nil
	}

	if !canTransition(record.State, to) {
		return ErrConflict
	}

	record.State = to

	return nil
}
