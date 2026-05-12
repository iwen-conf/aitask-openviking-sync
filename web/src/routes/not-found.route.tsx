import { Link } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'

export function NotFoundRoute() {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 bg-[hsl(var(--muted)/0.4)] p-6 text-center">
      <p className="font-mono text-sm uppercase tracking-widest text-[hsl(var(--muted-foreground))]">
        404
      </p>
      <h2 className="text-xl font-semibold text-[hsl(var(--foreground))]">页面不存在</h2>
      <Link
        to="/projects"
        className="inline-flex items-center gap-1 rounded-md bg-[hsl(var(--primary))] px-4 py-2 text-sm font-medium text-[hsl(var(--primary-foreground))] hover:bg-[hsl(var(--primary)/0.9)]"
      >
        <ArrowLeft className="h-4 w-4" /> 回到项目列表
      </Link>
    </div>
  )
}
