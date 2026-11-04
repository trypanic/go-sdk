exec(open('/tmp/p5-run.py').read().split('commands=')[0])
commands=[('floor-tagged-short','PATH=/home/trypanic/.go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.10.linux-amd64/bin:$PATH GOTOOLCHAIN=local go test -count=1 -tags integration -short ./...'),('tagged-staticcheck','staticcheck -tags integration ./...'),('tool-versions','go version; staticcheck -version; pre-commit --version; gcc --version; git diff --check db8d2c8..HEAD')]
for name,cmd in commands:
 start=time.time()
 with (out/(name+'.log')).open('w') as f:r=subprocess.run(cmd,shell=True,executable='/bin/bash',env=env,stdout=f,stderr=subprocess.STDOUT)
 entry=dict(name=name,command=cmd,exit=r.returncode,seconds=round(time.time()-start,2));(out/'extra.jsonl').open('a').write(json.dumps(entry)+'\n');print(json.dumps(entry),flush=True)
 if r.returncode:sys.exit(r.returncode)
