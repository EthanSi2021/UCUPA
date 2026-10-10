package commit

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	storebytes "ucupa/ucupa-core/internal/store"
)

// AttachmentHistoryForTest reports only the original assertions' event count
// and whether one specified operation has been retained. It returns no history.
func AttachmentHistoryForTest(owner *Owner, operation contract.OperationID) (int, bool) {
	owner.RLock()
	defer owner.RUnlock()
	return len(owner.state.attachments.events), owner.state.attachmentClaimed(operation)
}

// CorruptAttachmentStoreForTest retains the five original fixed journal faults.
// No database, writable state or arbitrary transaction callback is exported.
func CorruptAttachmentStoreForTest(path string, part string) error {
	switch part {
	case "outcome", "before-cut", "accepted-key", "contact", "commit":
	default:
		return fmt.Errorf("unknown attachment journal fault %q", part)
	}
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return err
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(elementBucketName)
		var events []storedAttachmentEvent
		if err := json.Unmarshal(bucket.Get(attachmentJournalKey), &events); err != nil {
			return err
		}
		switch part {
		case "outcome":
			events[1].Code = facetfact.Missing
		case "before-cut":
			events[1].Before++
		case "accepted-key":
			events[1].EvidenceKey.Element = values.ElementID("10000000-0000-4000-8000-0000000f1b93")
		case "contact":
			events[1].Attachment.NextHop = "local:forged"
		case "commit":
			return tx.Bucket(commitBucketName).Put(storebytes.EncodeUint64(events[1].After), []byte(`{"kind":"attachment","controlIndex":1}`))
		}
		raw, err := json.Marshal(events)
		if err != nil {
			return err
		}
		return bucket.Put(attachmentJournalKey, raw)
	})
	return errors.Join(err, db.Close())
}
