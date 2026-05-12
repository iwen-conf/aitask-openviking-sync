import * as React from 'react'
import { AlertOctagon } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface ErrorBoundaryState {
  error: Error | null
}

export class ErrorBoundary extends React.Component<
  { children: React.ReactNode },
  ErrorBoundaryState
> {
  constructor(props: { children: React.ReactNode }) {
    super(props)
    this.state = { error: null }
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: React.ErrorInfo): void {
    if (typeof console !== 'undefined') {
      console.error('[ErrorBoundary]', error, info)
    }
  }

  handleReset = () => {
    this.setState({ error: null })
  }

  render(): React.ReactNode {
    if (!this.state.error) return this.props.children
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-4 bg-slate-50 p-6 text-center">
        <AlertOctagon className="h-10 w-10 text-orange-500" />
        <div className="space-y-1">
          <p className="text-lg font-semibold text-slate-900">页面出现异常</p>
          <p className="max-w-md text-sm text-slate-500">
            {this.state.error.message || '请刷新页面或联系开发者排查。'}
          </p>
        </div>
        <Button variant="outline" onClick={this.handleReset}>
          重试
        </Button>
      </div>
    )
  }
}
