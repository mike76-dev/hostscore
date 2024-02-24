import { createContext } from 'react'

type NetworkContextType = {
    network: string,
    switchNetwork: (network: string) => any,
}

export const NetworkContext = createContext<NetworkContextType>({
    network: '',
    switchNetwork: (network: string) => {},
})