import { Component, PropsWithChildren } from 'react'
import './app.css'

// 小程序 App 入口（骨架）。登录态恢复、全局错误上报在 p2 接入。
class App extends Component<PropsWithChildren> {
  componentDidMount() {}

  render() {
    return this.props.children
  }
}

export default App
