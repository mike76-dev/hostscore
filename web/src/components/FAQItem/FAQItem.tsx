import './FAQItem.css'

type FAQItemProps = {
    parent: number,
    title: string,
    link: string
    expanded: boolean,
    expandItem: (link: string) => void,
    collapseItem: (parent: number) => void,
    children: React.ReactNode
}

export const FAQItem = (props: FAQItemProps) => {
    return (
        <div className={props.parent > 0 ? ' faq-item-subcontainer' : 'faq-item-container'}>
            <div
                className={'faq-item-title' + (props.expanded ? ' faq-item-title-expanded' : '')}
                onClick={() => {
                    if (props.expanded) props.collapseItem(props.parent)
                    else props.expandItem(props.link)
                }}
            >{props.title}</div>
            {props.expanded &&
                <div className={props.parent > 0 ? '' : 'faq-item-contents'}>{props.children}</div>
            }
        </div>
    )
}