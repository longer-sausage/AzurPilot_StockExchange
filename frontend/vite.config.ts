import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'

const apiURL=`http://127.0.0.1:${process.env.EXCHANGE_API_PORT??8888}`
export default defineConfig({plugins:[react()],server:{strictPort:true,proxy:{'/api':apiURL,'/healthz':apiURL}},build:{sourcemap:false}})
