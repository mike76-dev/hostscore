import './Sort.css'

type SortProps = {
    darkMode: boolean,
    order: 'asc' | 'desc' | 'none'
    setOrder: (order: 'asc' | 'desc') => any
}

export const Sort = (props: SortProps) => {
    const changeOrder = () => {
        props.setOrder(props.order === 'asc' ? 'desc' : 'asc')
    }
    return (
        <span className={'sort-container' + (props.darkMode ? ' sort-dark' : '')}>
            <svg
                width={16}
                height={16}
                viewBox="0 0 16 16"
                tabIndex={1}
                onClick={changeOrder}
                onKeyUp={(event: React.KeyboardEvent<SVGSVGElement>) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                        changeOrder()
                    }
                }}
            >
                <path className={
                    props.order === 'asc' ? 'sort-path-active' : 'sort-path'
                } d="M8 0 L12 6 L3 6 Z"/>
                <path className={
                    props.order === 'desc' ? 'sort-path-active' : 'sort-path'
                } d="M8 15 L12 9 L3 9 Z"/>
            </svg>
        </span>
    )
}