-- result_seen cursors are each person's (result_seen:<person>:<task>): the owner's, kept until now as result_seen:<task>, are named so.
UPDATE cursors SET name = 'result_seen:p1:' || substr(name, 13) WHERE name GLOB 'result_seen:[0-9]*';
