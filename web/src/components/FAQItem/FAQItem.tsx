import './FAQItem.css'

type FAQItemProps = {
    parent: number,
    index: number,
    title: string,
    expanded: boolean,
    expandItem: (parent: number, child: number) => void,
    children: React.ReactNode
}

export const FAQItem = (props: FAQItemProps) => {
    return (
        <div className={props.parent > 0 ? ' faq-item-subcontainer' : 'faq-item-container'}>
            <div
                className={'faq-item-title' + (props.expanded ? ' faq-item-title-expanded' : '')}
                onClick={() => {props.expandItem(props.parent, props.expanded ? 0 : props.index)}}
            >{props.title}</div>
            {props.expanded &&
                <div className={props.parent > 0 ? '' : 'faq-item-contents'}>{props.children}</div>
            }
        </div>
    )
}