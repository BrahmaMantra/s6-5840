~~~sh
fz@Brahmamantra:~/go/src/6.5840/src/main$ time bash test-mr.sh 
*** Starting wc test.
doFinishTask(): c.state  2
doFinishTask(): c.state  4
--- wc test: PASS
*** Starting indexer test.
doFinishTask(): c.state  2
doFinishTask(): c.state  4
--- indexer test: PASS
*** Starting map parallelism test.
doFinishTask(): c.state  2
doFinishTask(): c.state  4
--- map parallelism test: PASS
*** Starting reduce parallelism test.
doFinishTask(): c.state  2
doFinishTask(): c.state  4
--- reduce parallelism test: PASS
*** Starting job count test.
doFinishTask(): c.state  2
doFinishTask(): c.state  4
--- job count test: PASS
*** Starting early exit test.
doFinishTask(): c.state  2
doFinishTask(): c.state  4
--- early exit test: PASS
*** Starting crash test.
doFinishTask(): c.state  2
doFinishTask(): c.state  4
--- crash test: PASS
*** PASSED ALL TESTS
real    1m51.583s
user    0m16.044s
sys     0m4.291s
~~~