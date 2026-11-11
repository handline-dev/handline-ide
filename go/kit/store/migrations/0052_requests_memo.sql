-- requests_memo: the asks a comment raised, which requests.pending looks up for each unread @human comment.
CREATE INDEX IF NOT EXISTS requests_memo ON requests(memo_id) WHERE memo_id IS NOT NULL;
