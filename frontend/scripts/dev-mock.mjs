import {spawn} from 'node:child_process'
import {fileURLToPath} from 'node:url'
import {existsSync} from 'node:fs'
import {createServer} from 'vite'

const root=fileURLToPath(new URL('../../',import.meta.url))
const apiPort=Number(process.env.MOCK_API_PORT??8080)
if(!Number.isInteger(apiPort)||apiPort<1||apiPort>65535)throw new Error('MOCK_API_PORT 须为有效端口')
const apiURL=`http://127.0.0.1:${apiPort}`
const binary=process.env.EXCHANGE_BIN??`${root}bin/exchange${process.platform==='win32'?'.exe':''}`
const go=process.env.GO_BIN??'go'
const child=spawn(existsSync(binary)?binary:go,existsSync(binary)?[]:['run','./cmd/exchange'],{cwd:root,stdio:'inherit',env:{...process.env,MOCK_MODE:'true',DATABASE_PATH:process.env.MOCK_DATABASE??'data/mock.db',LISTEN_ADDR:`127.0.0.1:${apiPort}`,FRONTEND_DIR:'frontend/dist'}})
let server
const close=async()=>{await server?.close();if(process.platform==='win32'&&child.pid)spawn('taskkill',['/PID',String(child.pid),'/T','/F'],{stdio:'ignore'});else child.kill('SIGTERM');process.exit(0)}
process.on('SIGINT',close);process.on('SIGTERM',close);child.on('error',e=>{console.error(e.message);process.exit(1)});child.on('exit',code=>{if(code)process.exit(code)})
for(let attempts=0;attempts<300;attempts++){try{const r=await fetch(`${apiURL}/healthz`);if(r.ok)break}catch{}if(attempts===299)throw new Error('Mock 后端启动超时');await new Promise(r=>setTimeout(r,200))}
const meta=await (await fetch(`${apiURL}/api/meta`)).json();if(!meta.mock)throw new Error(`${apiPort} 端口不是 Mock 服务，请释放端口`)
server=await createServer({root:fileURLToPath(new URL('../',import.meta.url)),server:{host:'127.0.0.1',port:Number(process.env.MOCK_FRONTEND_PORT??5178),strictPort:true}})
await server.listen();server.printUrls();console.log('Mock 账户：海风指挥官 / mock-player-password；控制台：mock-admin-password；验证码使用 Google reCAPTCHA v2 官方测试组件与 www.recaptcha.net Siteverify，需要联网。')
