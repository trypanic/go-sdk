import os,subprocess,json,time,pathlib,sys
out=pathlib.Path('/tmp/P5-final');out.mkdir(exist_ok=True)
env=os.environ.copy();env['PATH']='/home/trypanic/.proto/tools/go/1.27.3/bin:'+env['PATH'];env['GOTOOLCHAIN']='local';env['GOFLAGS']='-count=1' if False else env.get('GOFLAGS','');env.pop('GOROOT',None)
commands=[('versions','go version; task --version; git rev-parse HEAD; git status --porcelain'),('format','gofmt -l .'),('unit','task test'),('coverage','go test -count=1 -short -coverprofile=/tmp/P5-final/coverage.out ./... && go tool cover -func=/tmp/P5-final/coverage.out'),('check','task check'),('lint','GOPROXY=file:///home/trypanic/.go/pkg/mod/cache/download GOSUMDB=off task lint'),('tagged-vuln','go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -tags=integration -test ./...'),('integration','task test:integration'),('tagged-no-docker','DOCKER_HOST=unix:///tmp/P5-no-docker.sock go test -count=1 -tags integration -short ./...'),('tagged-vet','go vet -tags integration ./...'),('hooks','task precommit:run'),('release-default','task release:check'),('release-comparison','task release:check RELEASE_VERSION=v2.0.0'),('release-initial','task release:check BASE=none RELEASE_VERSION=v2.0.0'),('clean','git status --porcelain; docker ps -a --filter label=org.testcontainers=true --format "{{.ID}} {{.Names}}"')]
for name,cmd in commands:
 start=time.time()
 with (out/(name+'.log')).open('w') as f:r=subprocess.run(cmd,shell=True,executable='/bin/bash',env=env,stdout=f,stderr=subprocess.STDOUT)
 entry=dict(name=name,command=cmd,exit=r.returncode,seconds=round(time.time()-start,2));(out/'checks.jsonl').open('a').write(json.dumps(entry)+'\n');print(json.dumps(entry),flush=True)
 if r.returncode and name not in ['release-default']:sys.exit(r.returncode)
