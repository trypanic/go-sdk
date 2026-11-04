import os,pathlib,subprocess,json,time,yaml
root=pathlib.Path.cwd();out=pathlib.Path('/tmp/P5-final');w=yaml.safe_load((root/'.github/workflows/ci.yml').read_text())
for line,tool in [('1.26.x','/home/trypanic/.go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.10.linux-amd64'),('1.27.x','/home/trypanic/.proto/tools/go/1.27.3')]:
 env=dict(os.environ);env.pop('GOROOT',None);env.update(PATH=tool+'/bin:'+env['PATH'],EXPECTED_GO_VERSION=line,GOVULNCHECK_VERSION=w['env']['GOVULNCHECK_VERSION'])
 for jobname in ['test','vuln']:
  env.update(w['jobs'][jobname]['env'])
  for i,step in enumerate(w['jobs'][jobname]['steps']):
   if 'run' not in step:continue
   name='matrix-'+line+'-'+jobname+'-'+str(i);cmd=step['run'];start=time.time()
   with (out/(name+'.log')).open('w') as f:r=subprocess.run(cmd,shell=True,executable='/bin/bash',env=env,stdout=f,stderr=subprocess.STDOUT)
   entry=dict(name=name,command=cmd,exit=r.returncode,seconds=round(time.time()-start,2));(out/'matrix.jsonl').open('a').write(json.dumps(entry)+'\n');print(json.dumps(entry),flush=True)
   if r.returncode:raise SystemExit(r.returncode)
