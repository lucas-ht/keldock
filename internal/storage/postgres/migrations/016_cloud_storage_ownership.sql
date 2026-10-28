-- Several projects may observe an existing bucket, but only one intent may own
-- its mutable metadata. Disconnection retains ownership history until operator review.
CREATE UNIQUE INDEX one_cloud_storage_writer ON cloud_intents(external_key)
  WHERE kind='resource' AND external_key<>'';
