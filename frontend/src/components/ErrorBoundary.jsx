import { Component } from 'react'

export class ErrorBoundary extends Component {
  state = { error: null }

  static getDerivedStateFromError(error) { return { error } }

  render() {
    if (this.state.error) {
      return <main className="react-screen-center">
        <div className="react-fatal">
          <span className="dashboard-eyebrow">Application error</span>
          <h1>This view stopped unexpectedly</h1>
          <p>{this.state.error.message}</p>
          <button className="btn btn-primary" type="button" onClick={() => window.location.reload()}>Reload application</button>
        </div>
      </main>
    }
    return this.props.children
  }
}
