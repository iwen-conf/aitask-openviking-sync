import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import { i18nReady } from './i18n'
import App from './App.tsx'

const container = document.getElementById('root')!

// 等待 i18n 就绪再挂载,避免首屏闪现翻译 key (`nav.workspace` 等);resources 内联,通常在
// 当前 microtask 即解析。
void i18nReady.then(() => {
  createRoot(container).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
})
