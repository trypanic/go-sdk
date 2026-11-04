import subprocess,time,json,pathlib
out=pathlib.Path('/tmp/DEPS-review1');found={}
with (out/'ryuk-test.log').open('w') as log:
 p=subprocess.Popen(['go','test','-v','-tags','integration','-run','TestRabbitMQIntegration/mandatory','-count=1','-timeout=3m','./messaging'],cwd='/tmp/go-sdk-audit-DEPS',stdout=log,stderr=subprocess.STDOUT)
 while p.poll() is None:
  ids=subprocess.check_output(['docker','ps','-aq','--filter','ancestor=testcontainers/ryuk:0.14.0']).decode().split()
  for id in ids:
   r=subprocess.run(['docker','inspect',id],capture_output=True,text=True)
   if r.returncode:continue
   c=json.loads(r.stdout)[0]
   found[id]={'image':c['Config']['Image'],'ports':c['NetworkSettings']['Ports'],'privileged':c['HostConfig']['Privileged'],'mounts':c['HostConfig']['Binds']}
  time.sleep(.1)
print('test exit',p.returncode)
(out/'ryuk-bindings.json').write_text(json.dumps(found,indent=2)+'\n');print(json.dumps(found,indent=2));assert found
raise SystemExit(p.returncode)
