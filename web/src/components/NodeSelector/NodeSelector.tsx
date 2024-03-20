import { ChangeEvent } from 'react'
import './NodeSelector.css'

type NodeSelectorProps = {
    darkMode: boolean,
    nodes: string[],
    node: string,
    setNode: (node: string) => any,
}

export const NodeSelector = (props: NodeSelectorProps) => {
    const onChange = (e: ChangeEvent<HTMLSelectElement>): any => {
        props.setNode(e.target.value)
    }
    return (
        <div className={'node-selector-container' + (props.darkMode ? ' node-selector-dark' : '')}>
            <label>
                <span className="node-selector-text">Select node:</span>
                <select
                    className="node-selector-select"
                    tabIndex={1}
                    onChange={onChange}
                >
                    {props.nodes.map((n, i) => (
                        <option
                            key={n}
                            value={n}
                        >{i === 0 ? 'Global' : n.toUpperCase()}</option>
                    ))}
                </select>
            </label>
        </div>
    )
}
