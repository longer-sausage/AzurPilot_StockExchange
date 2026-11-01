import {createContext,useContext,useEffect,useState,type ReactNode} from 'react'
import {Moon,Sun} from 'lucide-react'
type Theme='light'|'dark'
const ThemeContext=createContext<{theme:Theme;toggle:()=>void}>({theme:'light',toggle:()=>{}})
export function ThemeProvider({children}:{children:ReactNode}){
 const [theme,setTheme]=useState<Theme>(()=>{try{const saved=localStorage.getItem('mmex-theme');if(saved==='dark'||saved==='light')return saved}catch{}return matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'})
 useEffect(()=>{document.documentElement.dataset.stockTheme=theme;document.documentElement.style.colorScheme=theme;try{localStorage.setItem('mmex-theme',theme)}catch{}},[theme])
 return <ThemeContext.Provider value={{theme,toggle:()=>setTheme(v=>v==='dark'?'light':'dark')}}>{children}</ThemeContext.Provider>
}
export const useStockTheme=()=>useContext(ThemeContext)
export function ThemeToggle(){const {theme,toggle}=useStockTheme();return <button className="icon" aria-label={theme==='dark'?'切换亮色主题':'切换暗色主题'} onClick={toggle}>{theme==='dark'?<Sun size={18}/>:<Moon size={18}/>}</button>}
