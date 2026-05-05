import { registerRootComponent } from 'expo';
import App from './App';

// registerRootComponent llama a AppRegistry.registerComponent('main', ...)
// y asegura que el entorno está configurado correctamente (Expo Go, standalone, web).
registerRootComponent(App);
