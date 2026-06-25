import { createApp } from 'vue'
import App from './App.vue'
import { installFrontendErrorLogging } from './errorLogger'
import './style.css'

const app = createApp(App)
installFrontendErrorLogging(app)
app.mount('#app')
